package genserver

import (
	"log"
	"time"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
)

// gencaller returns reply from genserver process or times out
type genCaller struct {
	out     chan any
	gensrv  erl.PID
	tout    time.Duration
	request any
}

func (gc *genCaller) Receive(self erl.PID, inbox <-chan any) error {
	// monitor the genserver so we can exit if it crashes
	ref := erl.Monitor(self, gc.gensrv)

	// make the call to the genserver. It will reply to us with a [CallReply], [CallReturnStatus] indicates
	// whether this was succesful or not
	erl.Send(gc.gensrv, callRequest{from: From{caller: self, mref: ref}, term: gc.request})

	for {
		select {
		case msg, ok := <-inbox:
			if !ok {
				return nil
			}
			switch msgT := msg.(type) {

			case callReply:
				gc.out <- msgT
				return exitreason.Normal

			case erl.DownMsg:
				log.Printf("genCaller got DOWN msg: %+v", msgT)
				if exitreason.IsNormal(msgT.Reason) || exitreason.IsShutdown(msgT.Reason) {
					gc.out <- callReply{Status: Stopped, Term: msgT}
				} else {
					gc.out <- callReply{Status: Other, Term: msgT}
				}

			}
		case <-time.After(gc.tout):
			gc.out <- callReply{Status: Timeout}
			return exitreason.Normal
		}
	}
}
