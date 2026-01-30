package erl

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"runtime/trace"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/uberbrodt/fungo/fun"

	"github.com/uberbrodt/erl-go/erl/exitreason"
	"github.com/uberbrodt/erl-go/erl/internal/inbox"
)

var nextProcessID atomic.Int64

// this stores monitors that other processes take on a process
type pMonitor struct {
	pid PID
	ref Ref
}

type spawnMonitor struct {
	pid PID
	ref Ref
}

type Process struct {
	id              int64
	runnable        Runnable
	receive         *inbox.Inbox[Signal] // chan Signal
	runnableReceive *inbox.Inbox[any]
	done            chan error
	links           []PID
	monitors        []pMonitor
	cleanupMonitors []pMonitor
	monitoring      map[Ref]PID
	_status         processStatus
	exitReason      *exitreason.S
	_trapExits      bool
	statusMutex     sync.RWMutex
	nameMutex       sync.RWMutex
	trapMutex       sync.RWMutex
	// the local name of the pid, optional
	_name Name
	// if set, process sets itself as monitored by PID before anything else
	spawnMonitor *spawnMonitor
	// if set, process sets a link with PID before anything else
	spawnLink *PID
	taskCtx   context.Context
	traceTask *trace.Task
}

func (p *Process) String() string {
	if p.getName() != "" {
		return fmt.Sprintf("Process<%d|%s>", p.id, p.getName())
	} else {
		return fmt.Sprintf("Process<%d>", p.id)
	}
}

func (p *Process) run() {
	p.taskCtx, p.traceTask = trace.NewTask(context.Background(), "erl.process")
	defer p.traceTask.End()
	// if set, this was created via [SpawnMonitor], and we MUST set the monitor before we start processing the inbox
	// so that if the Runnable exits immediately, the [DownMsg] will be sent
	if p.spawnMonitor != nil {
		DebugPrintf("[%+v] setting spawn monitor for %+v", p.self(), p.spawnMonitor.pid)
		monitorSig := monitorSignal{ref: p.spawnMonitor.ref, monitor: p.spawnMonitor.pid, monitored: p.self()}
		sendSignal(p.spawnMonitor.pid, monitorSig)
		p.monitors = append(p.monitors, pMonitor{pid: p.spawnMonitor.pid, ref: p.spawnMonitor.ref})
	}

	if p.spawnLink != nil {
		DebugPrintf("[%+v] setting spawn link for %+v", p.self(), *p.spawnLink)
		sendSignal(*p.spawnLink, linkSignal{pid: p.self()})
		p.links = append(p.links, *p.spawnLink)

	}

	trace.Log(p.taskCtx, "erl.PID", p.self().String())

	// start a go routine that will handle the [Runnable] and feed it [messageSignal]s
	go func() {
		trace.WithRegion(p.taskCtx, "erl.process.runnable", func() {
			trace.Log(p.taskCtx, "erl.PID", p.self().String())

			var result error
			defer func() {
				// if the [Runnable] panics, we log it and put an Exception reason on the done channel
				if r := recover(); r != nil {
					e, ok := r.(error)
					if ok {
						result = fmt.Errorf("%v Runnable.Receive panicked: %w, stack: %+v", p, e, string(debug.Stack()))
					} else {
						result = fmt.Errorf("%v Runnable.Receive panicked: %s: stack: %+v", p, r, string(debug.Stack()))
					}
					Logger.Println(result)
					p.done <- exitreason.Exception(result)

				}
			}()

			runnableExitReason := p.runnable.Receive(PID{p: p}, p.runnableReceive.Channel())
			if runnableExitReason == nil {
				runnableExitReason = exitreason.Normal
			}
			if result != nil {
				Logger.Printf("%v runnable exited with error: %s\r", p, result)
			} else {
				DebugPrintf("%v runnable exited\r", p)
			}
			p.done <- runnableExitReason
			close(p.done)
		})
	}()
	sigChannel := p.receive.Channel()
	for {
		select {
		// this happens when the Runnable exits
		case exitReason := <-p.done:
			// if we're in status running, it means the runnable exited, so we need to
			// perform an exit.
			DebugPrintf("Runnable exited")
			if p.getStatus() == running {
				p.exit(exitReason)
			}
			return

		// We got a signal from another process
		case signal, open := <-sigChannel:
			var stop bool
			trace.WithRegion(p.taskCtx, "erl.process.signalhandler", func() {
				if !open {
					DebugPrintf("%v process got inbox closed, should have exited before this.", p)
					stop = true
					return
				}
				DebugPrintf("%v received %s signal\r", p.self(), signal.SignalName())
				switch sig := signal.(type) {

				case monitorSignal:
					p.handleMonitorSignal(sig)
				case demonitorSignal:
					p.handleDemonitorSignal(sig)

				case linkSignal:
					p.handleLinkSignal(sig)
				case unlinkSignal:
					p.handleUnlinkSignal(sig)
				// all message signals go to the runnable
				case messageSignal:
					if p.getStatus() == running {
						p.runnableReceive.Enqueue(sig.term)
					}
				case downSignal:
					if p.getStatus() == running {

						_, ok := p.monitoring[sig.ref]
						if !ok {
							Logger.Printf("%v, got a DOWN signal but could not match Ref %+v\r", p.self(), sig)
							break
						}
						// convert to an [erl.DownMsg] and send to the Runnable
						p.runnableReceive.Enqueue(downMsgfromSignal(sig))
						// for custom message types
						// this should happen as a result of a linked process existing or someone calling [Exit] on the process.
					}
				case exitSignal:
					if p.getStatus() == running {
						// can't trap Kill if it's sent to us, but if a linked process exited with reason Kill,
						// then we can still trap the exit below
						if errors.Is(sig.reason, exitreason.Kill) && !sig.link {
							p.exit(sig.reason)
							stop = true
							return
						}

						// if we're trapping exits, send the signal to the runnable for them to deal with, don't exit.
						if p.trapExits() {
							DebugPrintf("%+v Trapped exit signal from %+v", p.self(), sig.sender)
							p.runnableReceive.Enqueue(exitMsgFromSignal(sig))
							DebugPrintf("%v sent exitMsg", p.self())
						} else {
							// ignore normal exits from other processes when not trapping exits; [exitreason.Normal] is
							// a valid way to exit from within a process, but an external process can't generate an
							// exitsignal with it.
							if errors.Is(sig.reason, exitreason.Normal) && sig.link && !sig.sender.Equals(p.self()) {
								// continue
								return
							}
							p.exit(sig.reason)
							stop = true
							return
						}
					}
				}
			})
			if stop {
				return
			}

		}
	}
}

func (p *Process) handleMonitorSignal(sig monitorSignal) {
	if p.getStatus() != running {
		sig.monitor.p.send(downSignal{proc: p.self(), ref: sig.ref, reason: exitreason.NoProc})
		return
		// continue
	}
	// this is a monitor we took of another process
	if sig.monitor.Equals(p.self()) {
		p.monitoring[sig.ref] = sig.monitored
	} else {
		// this is a monitor another process took of us
		p.monitors = append(p.monitors, pMonitor{pid: sig.monitor, ref: sig.ref})
	}
}

func (p *Process) handleDemonitorSignal(sig demonitorSignal) {
	if sig.origin.Equals(p.self()) {
		monitoredPid := p.monitoring[sig.ref]
		sendSignal(monitoredPid, sig)
		delete(p.monitoring, sig.ref)
	} else {
		p.monitors = fun.Filter(p.monitors, func(v pMonitor) bool {
			return v.ref != sig.ref && v.pid != sig.origin
		})
	}
}

func (p *Process) handleLinkSignal(sig linkSignal) {
	if p.getStatus() != running {
		sig.pid.p.send(exitSignal{sender: p.self(), receiver: sig.pid, reason: exitreason.NoProc, link: true})
		// continue
		return
	}
	if !slices.Contains(p.links, sig.pid) {
		p.links = append(p.links, sig.pid)
	}
}

func (p *Process) handleUnlinkSignal(sig unlinkSignal) {
	idx := slices.Index(p.links, sig.pid)
	if idx != -1 {
		p.links = slices.Delete(p.links, idx, idx+1)
	} else {
		Logger.Printf("%v received an unlink signal for %+v, but one could not be found\r", p.self(), sig.pid)
	}
}

func (p *Process) exit(e error) {
	trace.WithRegion(p.taskCtx, "erl.process.exit", func() { // set status to exiting so that our main loop doesn't send signals twice.
		DebugPrintf("process %v exiting...", p)
		p.setStatus(exiting)
		if p.getName() != "" {
			DebugPrintf("%v unregistering name: %s", p, p.getName())
			Unregister(p.getName())

		}

		// now that no more messages are getting onto the inbox, read the whole thing
		// and process any monitor, demonitor, link and unlink signals so that
		// anything that called Monitor or Link on this process after we got a signal to exit
		// and before we setStatus to exiting will get the signals they expect.
		for _, signal := range p.receive.Drain() {
			fmt.Printf("got a signal after exit: %#v\n", signal)

			switch sig := signal.(type) {

			case monitorSignal:
				fmt.Printf("handling post-exit monitoringSignal: %#v\n", sig)
				if sig.monitor.Equals(p.self()) {
					p.monitoring[sig.ref] = sig.monitored
				} else {
					// this is a monitor another process took of us
					p.monitors = append(p.monitors, pMonitor{pid: sig.monitor, ref: sig.ref})
				}
			case demonitorSignal:
				fmt.Printf("handling post-exit demonitorSignal: %#v\n", sig)
				p.handleDemonitorSignal(sig)
			case linkSignal:
				fmt.Printf("handling post-exit linkSignal: %#v\n", sig)
				if !slices.Contains(p.links, sig.pid) {
					p.links = append(p.links, sig.pid)
				}
			case unlinkSignal:
				fmt.Printf("handling post-exit unlinkSignal: %#v \n", sig)
				p.handleUnlinkSignal(sig)
			default:
				// ignore
			}
		}

		var exitReason *exitreason.S
		if e == nil {
			exitReason = exitreason.Normal
		}

		if !errors.As(e, &exitReason) {
			tmpE := exitreason.Exception(e)
			errors.As(tmpE, &exitReason)
		}

		for _, linked := range p.links {
			linked.p.send(exitSignal{sender: p.self(), receiver: linked, reason: exitReason, link: true})
		}
		// notify processes monitoring us that we're down
		for _, monit := range p.monitors {
			monit.pid.p.send(downSignal{proc: p.self(), ref: monit.ref, reason: exitReason})
		}
		// notify processes we're monitoring that we're not monitoring them anymore
		// this is important to make sure this process is GC'd
		for ref, monitPID := range p.monitoring {
			monitPID.p.send(demonitorSignal{ref: ref, origin: p.self()})
		}

		p.exitReason = exitReason
		DebugPrintf("%v closing runnableReceive", p)
		p.runnableReceive.Close()
		// wait until runnable has exited
		<-p.done
		p.setStatus(exited)
		p.receive.Close()
		// null out the maps so that we don't hold references that would
		// prevent garbage collection of finished processes.
		p.links = nil
		p.monitors = nil
		p.monitoring = nil
	})
}

func (p *Process) self() PID {
	return PID{p: p}
}

// we never close receive channel but we do stop reading from it, which causes a deadlock
// So if the process is no longer running, handle cases where we need to send back an Exit or
// Down message, but otherwise no-op
func (p *Process) send(sig Signal) {
	if p.getStatus() == running {
		p.receive.Enqueue(sig)
		return
	}

	// one of the guarantees of [erl] is that you'll always got a DownMsg/ExitMsg if you link to a process
	// even if it's already dead.
	switch signal := sig.(type) {
	case linkSignal:
		signal.pid.p.send(exitSignal{sender: p.self(), receiver: signal.pid, reason: exitreason.NoProc, link: true})
	case monitorSignal:
		signal.monitor.p.send(downSignal{proc: p.self(), ref: signal.ref, reason: exitreason.NoProc})
	default:
		// just ignore

	}
}

func (p *Process) setTrapExits(v bool) {
	defer p.trapMutex.Unlock()
	p.trapMutex.Lock()

	p._trapExits = v
}

func (p *Process) trapExits() bool {
	defer p.trapMutex.RUnlock()
	p.trapMutex.RLock()
	return p._trapExits
}

func (p *Process) getStatus() processStatus {
	defer func() {
		if p != nil {
			p.statusMutex.RUnlock()
		}
	}()
	if p == nil {
		return exited
	}
	p.statusMutex.RLock()
	return p._status
}

func (p *Process) setStatus(newStatus processStatus) {
	p.statusMutex.Lock()
	defer p.statusMutex.Unlock()

	p._status = newStatus
}

func (p *Process) getName() Name {
	p.nameMutex.RLock()
	defer p.nameMutex.RUnlock()
	return p._name
}

func (p *Process) setName(name Name) {
	p.nameMutex.Lock()
	defer p.nameMutex.Unlock()

	p._name = name
}

// tryRegisterName atomically checks that the process is running and has no name,
// then sets the name. This prevents a TOCTOU race between Register checking
// IsAlive and the process exiting before setName is called.
//
// Returns nil on success, or a RegistrationError if:
//   - Process is not running (NoProc)
//   - Process already has a name (AlreadyRegistered)
func (p *Process) tryRegisterName(name Name) *RegistrationError {
	// Lock order: status first, then name (consistent with exit())
	p.statusMutex.RLock()
	defer p.statusMutex.RUnlock()

	if p._status != running {
		return &RegistrationError{Kind: NoProc}
	}

	p.nameMutex.Lock()
	defer p.nameMutex.Unlock()

	if p._name != "" {
		return &RegistrationError{Kind: AlreadyRegistered}
	}

	p._name = name
	return nil
}

func NewProcess(r Runnable) *Process {
	return &Process{
		id:              nextProcessID.Add(1),
		runnable:        r,
		receive:         inbox.New[Signal](),
		runnableReceive: inbox.New[any](),
		done:            make(chan error),
		links:           make([]PID, 0),
		monitors:        make([]pMonitor, 0),
		monitoring:      make(map[Ref]PID),
		_status:         running,
	}
}
