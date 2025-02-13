package genserver

import (
	"time"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
)

/*
This is used when the caller of Start* is trapping exits; we want to synchronously start and not send
down or exit messages to the parent. This makes supervisors easier to reason about.

- genStarter traps exits and starts the process and swaps the parent with [parent], like [doStart]
- if it gets an error on initAck channel, then it waits for the exit messages and returns the errors
  on the result channel, this is then sent to the parent
-



*/

type genStarter[STATE any] struct {
	gensrv *GenServerS[STATE]
	parent erl.PID
	sig    chan<- struct{}
	tout   time.Duration
	gsPID  chan<- erl.PID
}

// send this to a genStarter when it's genserver init'd successfully and is no longer needed
type genStarterShutdown struct{}

func (gs *genStarter[STATE]) Receive(self erl.PID, inbox <-chan any) error {
	// monitor the genserver so we can exit if it crashes
	erl.ProcessFlag(self, erl.TrapExit, true)

	// link ourselves to the parent process so that we receive an exit msg if it crashes.
	parentRef := erl.Monitor(self, gs.parent)
	gensrvPID, ref := erl.SpawnMonitor(self, gs.gensrv)
	gs.gsPID <- gensrvPID

	for {
		select {
		case anymsg, ok := <-inbox:
			if !ok {
				return exitreason.Normal
			}
			switch msg := anymsg.(type) {
			case genStarterShutdown:
				return exitreason.Normal
			case erl.DownMsg:
				if msg.Ref == ref && msg.Proc.Equals(gensrvPID) {
					close(gs.sig)
					return exitreason.Normal
				}

				if msg.Ref == parentRef && msg.Proc.Equals(gs.parent) {
					Stop(self, gensrvPID, StopReason(exitreason.Kill))
					return exitreason.Normal
				}
			}

		case <-time.After(gs.tout):
			// close the sig so that we don't block the caller.
			close(gs.sig)
			return exitreason.Normal
		}
	}
}
