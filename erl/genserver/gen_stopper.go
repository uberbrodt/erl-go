package genserver

import (
	"errors"
	"time"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
)

// genStopper handles graceful GenServer shutdown for the Stop function.
// It monitors the target GenServer and sends either a stopRequest (for graceful
// shutdown with Terminate callback) or an exit signal (for Kill).
type genStopper struct {
	caller     erl.PID
	out        chan *exitreason.S
	gensrv     erl.PID
	tout       time.Duration
	exitReason *exitreason.S
}

func (gc *genStopper) Receive(self erl.PID, inbox <-chan any) error {
	erl.DebugPrintf("genStopper[%v]: preparing to stop %v", self, gc.gensrv)
	// monitor the genserver so we can detect when it exits
	ref := erl.Monitor(self, gc.gensrv)

	// exitreason.Kill bypasses the Terminate callback (Erlang behavior),
	// so we use a direct exit signal. For all other reasons, we send
	// a stopRequest message that triggers the Terminate callback.
	if errors.Is(gc.exitReason, exitreason.Kill) {
		erl.Exit(gc.caller, gc.gensrv, gc.exitReason)
	} else {
		erl.Send(gc.gensrv, stopRequest{reason: gc.exitReason})
	}

	for {
		select {
		case msg := <-inbox:
			switch msgT := msg.(type) {
			case erl.DownMsg:
				if msgT.Ref == ref {
					erl.DebugPrintf("genStopper[%v]: got DownMsg for  %v", self, gc.gensrv)
					gc.out <- msgT.Reason
					return exitreason.Normal
				} else {
					erl.Logger.Printf("genStopper[%v] got DownMsg with ref: %v, but looking for ref: %v", self, msgT.Ref, ref)
				}
			default:
				erl.Logger.Printf("genStopper[%v]: received unknown msg: %+v", self, msgT)

			}
		case <-time.After(gc.tout):
			erl.DebugPrintf("genStopper[%v]: timed out", self)
			gc.out <- exitreason.Timeout
			return exitreason.Normal
		}
	}
}
