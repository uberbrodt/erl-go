package erl

import (
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/exp/slices"

	"github.com/uberbrodt/fungo/fun"

	"github.com/uberbrodt/erl-go/chronos"
	"github.com/uberbrodt/erl-go/erl/exitreason"
	"github.com/uberbrodt/erl-go/erl/internal/inbox"
)

var nextProcessID atomic.Int64

type pMonitor struct {
	pid PID
	ref Ref
}

type Process struct {
	id              int64
	runnable        Runnable
	receive         chan Signal
	runnableReceive *inbox.Inbox[any]
	done            chan error
	links           []PID
	monitors        []pMonitor
	monitoring      map[Ref]PID
	_status         processStatus
	exitReason      *exitreason.S
	_trapExits      bool
	statusMutex     sync.RWMutex
	nameMutex       sync.RWMutex
	trapMutex       sync.RWMutex
	// the local name of the pid, optional
	_name Name
}

func (p *Process) String() string {
	if p.getName() != "" {
		return fmt.Sprintf("Process<%d|%s>", p.id, p.getName())
	} else {
		return fmt.Sprintf("Process<%d>", p.id)
	}
}

func (p *Process) run() {
	go func() {
		var result error
		defer func() {
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
		runnableExitReason := p.runnable.Receive(PID{p: p}, p.runnableReceive.Receive())
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
	}()
	for {
		select {
		// this happens when the Runnable exits
		case exitReason := <-p.done:
			// if we're in status running, it means the runnable exited, so we need to
			// perform an exit.
			if p.getStatus() == running {
				p.exit(exitReason)
			}
			return

		case signal := <-p.receive:
			DebugPrintf("%v received %s signal\r", p.self(), signal.SignalName())
			switch sig := signal.(type) {

			case monitorSignal:
				if p.getStatus() != running {
					sig.monitor.p.send(downSignal{proc: p.self(), ref: sig.ref, reason: exitreason.NoProc})
					continue
				}
				if sig.monitor == p.self() {
					p.monitoring[sig.ref] = sig.monitored
				} else {
					// log.Info().Msgf("Monitors taken: %+v", b)
					p.monitors = append(p.monitors, pMonitor{pid: sig.monitor, ref: sig.ref})
				}
			case demonitorSignal:
				if sig.origin == p.self() {
					monitoredPid := p.monitoring[sig.ref]
					sendSignal(monitoredPid, sig)
					delete(p.monitoring, sig.ref)
				} else {
					p.monitors = fun.Filter(p.monitors, func(v pMonitor) bool {
						return v.ref != sig.ref && v.pid != sig.origin
					})
				}

			case linkSignal:
				if p.getStatus() != running {
					sig.pid.p.send(exitSignal{sender: p.self(), receiver: sig.pid, reason: exitreason.NoProc, link: true})
					continue
				}
				if !slices.Contains(p.links, sig.pid) {
					p.links = append(p.links, sig.pid)
				}
			case unlinkSignal:
				idx := slices.Index(p.links, sig.pid)
				if idx != -1 {
					p.links = slices.Delete(p.links, idx, idx+1)
				} else {
					Logger.Printf("%v received an unlink signal for %+v, but one could not be found\r", p.self(), sig.pid)
				}

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
							continue
						}
						p.exit(sig.reason)
						return
					}
				}
			}

		}
	}
}

func (p *Process) exit(e error) {
	// set status to exiting so that our main loop doesn't send signals twice.
	p.setStatus(exiting)
	if p.getName() != "" {
		DebugPrintf("%v unregistering name: %s", p, p.getName())
		Unregister(p.getName())

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
	for _, monit := range p.monitors {
		monit.pid.p.send(downSignal{proc: p.self(), ref: monit.ref, reason: exitReason})
	}
	p.exitReason = exitReason
	p.runnableReceive.Close()
	// wait until runnable has exited
	<-p.done
	p.setStatus(exited)
}

func (p *Process) self() PID {
	return PID{p: p}
}

// we never close receive channel but we do stop reading from it, which causes a deadlock
// So if the process is no longer running, handle cases where we need to send back an Exit or
// Down message, but otherwise no-op
func (p *Process) send(sig Signal) {
	if p.getStatus() == running {
		var notRunning bool
		for !notRunning {
			select {
			case p.receive <- sig:
				return
			case <-time.After(chronos.Dur("5ms")):
				notRunning = p.getStatus() != running
			}
		}
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

func NewProcess(r Runnable) *Process {
	return &Process{
		id:              nextProcessID.Add(1),
		runnable:        r,
		receive:         make(chan Signal),
		runnableReceive: inbox.New[any](),
		done:            make(chan error),
		links:           make([]PID, 0),
		monitors:        make([]pMonitor, 0),
		monitoring:      make(map[Ref]PID),
		_status:         running,
	}
}
