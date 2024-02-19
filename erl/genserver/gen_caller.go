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
	// monitor ref for genserver
	gsRef erl.Ref
}

func (gc *genCaller) Receive(self erl.PID, inbox <-chan any) error {
	// monitor the genserver so we can exit if it crashes
	gc.gsRef = erl.Monitor(self, gc.gensrv)

	// make the call to the genserver. It will reply to us with a [CallReply], [CallReturnStatus] indicates
	// whether this was succesful or not
	erl.Send(gc.gensrv, CallRequest{From: From{caller: self, mref: gc.gsRef}, Msg: gc.request})

	for {
		select {
		case msg, ok := <-inbox:
			if !ok {
				return exitreason.Normal
			}
			if err := gc.handleMsg(self, msg); err != nil {
				return err
			}
		case <-time.After(gc.tout):
			gc.out <- CallReply{Status: Timeout}
			return exitreason.Normal
		}
	}
}

func (gc *genCaller) handleMsg(self erl.PID, anymsg any) error {
	switch msg := anymsg.(type) {

	case CallReply:
		gc.out <- msg
		return exitreason.Normal

	case erl.DownMsg:
		if msg.Ref == gc.gsRef {
			log.Printf("genCaller got DOWN msg from genserver: %+v", msg)
			if exitreason.IsNormal(msg.Reason) || exitreason.IsShutdown(msg.Reason) {
				gc.out <- CallReply{Status: Stopped, Term: msg}
			} else {
				gc.out <- CallReply{Status: Other, Term: msg}
			}
			return exitreason.Normal
		} else {
			log.Printf("genCaller got DOWN msg from an unknown process: %+v", msg)
			return nil
		}
	default:
		log.Printf("genCaller %v received unhandled message: %s", self, msg)
		return nil
	}
}
