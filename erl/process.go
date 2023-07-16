package erl

import (
	"github.com/rs/zerolog/log"
	"golang.org/x/exp/slices"

	"github.com/uberbrodt/fungo/fun"
)

type pMonitor struct {
	pid PID
	ref Ref
}

type Process struct {
	runnable        Runnable
	receive         chan Signal
	runnableReceive chan any
	done            chan error
	links           []PID
	monitors        []pMonitor
	monitoring      map[Ref]PID
	status          processStatus
	exitReason      string
	trapExits       bool
}

func (p *Process) run() {
	go func() {
		result := p.runnable.Receive(PID{p: p}, p.runnableReceive)
		log.Debug().Err(result).Msg("Process runnable exited")
		p.done <- result
	}()
	for {
		if p.status == running {
			select {
			// this happens when the Runnable exits
			case exitReason := <-p.done:
				p.exit(exitReason)
				return

			case signal := <-p.receive:
				log.Debug().Msgf("Process received %s signal", signal.SignalName())
				switch sig := signal.(type) {
				case monitorSignal:
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
					if !slices.Contains(p.links, sig.pid) {
						p.links = append(p.links, sig.pid)
					}
				case unlinkSignal:
					idx := slices.Index(p.links, sig.pid)
					if idx != -1 {
						p.links = slices.Delete(p.links, idx, idx+1)
					} else {
						log.Debug().Msgf("received an unlink signal for %+v, but one could not be found", sig.pid)
					}

				case messageSignal:
					p.runnableReceive <- sig.term
				case downSignal:

					_, ok := p.monitoring[sig.Ref]
					if !ok {
						log.Warn().Msgf("Got a DOWN signal but could not match Ref %+v", sig)
						break
					}
					p.runnableReceive <- DownMsg(sig)
					// for custom message types
					// this should happen as a result of a linked process existing or someone calling [Exit] on the process.
				case exitSignal:
					// if we're trapping exits, send the signal to the runnable for them to deal with, don't exit.
					if p.trapExits {
						p.runnableReceive <- ExitMsg(sig)
					} else {
						p.exit(sig.reason)
						return
					}
				}

			}
		}
	}
}

func (p *Process) exit(exitReason error) {
	p.status = exiting

	for _, linked := range p.links {
		if linked.p.status == running {
			linked.p.receive <- exitSignal{proc: PID{p: p}, reason: exitReason, link: true}
		}
	}
	for _, monit := range p.monitors {
		if monit.pid.p.status == running {
			monit.pid.p.receive <- downSignal{Proc: PID{p: p}, Ref: monit.ref, Reason: exitReason}
		}
	}
	if exitReason == nil {
		p.exitReason = "normal"
	} else {
		p.exitReason = exitReason.Error()
	}
	close(p.runnableReceive)
	p.status = exited
}

func (p *Process) self() PID {
	return PID{p: p}
}

func NewProcess(r Runnable) *Process {
	return &Process{
		runnable: r,
		receive:  make(chan Signal),
		// XXX: Buffering this simplifies the Process code but ultimately will need to be replaced.
		// The issue is that [Send] will block when the buffer fills up and the intention of Send is
		// to NEVER block.
		runnableReceive: make(chan any, 10_000),
		done:            make(chan error),
		links:           make([]PID, 0),
		monitors:        make([]pMonitor, 0),
		monitoring:      make(map[Ref]PID),
		status:          running,
	}
}
