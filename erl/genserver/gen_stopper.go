package genserver

import (
	"time"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
)

// gencaller returns reply from genserver process or times out
type genStopper struct {
	caller     erl.PID
	out        chan *exitreason.S
	gensrv     erl.PID
	tout       time.Duration
	exitReason *exitreason.S
}

func (gc *genStopper) Receive(self erl.PID, inbox <-chan any) error {
	erl.DebugPrintf("genStopper[%v]: preparing to stop %v", self, gc.gensrv)
	// monitor the genserver so we can exit if it crashes
	ref := erl.Monitor(self, gc.gensrv)

	// This is kind of a hack. We need to simulate a parent process calling Exit(child),
	// in order for GenServers to exit and call HandleTerminate
	erl.Exit(gc.caller, gc.gensrv, gc.exitReason)

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
