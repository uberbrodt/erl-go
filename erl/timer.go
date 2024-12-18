package erl

import (
	"time"

	"github.com/uberbrodt/erl-go/erl/exitreason"
)

type TimerRef struct {
	pid PID
}

func CancelTimer(tr TimerRef) error {
	if tr.pid.IsNil() || tr.pid == UndefinedPID {
		return exitreason.NoProc
	}
	Send(tr.pid, cancelTimer{})
	return nil
}

type timer struct {
	to   PID
	term any
	tout time.Duration
	ref  Ref
}

type cancelTimer struct{}

func (t *timer) Receive(self PID, inbox <-chan any) error {
	t.ref = Monitor(self, t.to)

	for {
		select {
		case anyMsg, ok := <-inbox:
			if !ok {
				return exitreason.Normal
			}
			switch msg := anyMsg.(type) {
			case cancelTimer:
				DebugPrintf("<timer:%v> cancelled", self)
				return exitreason.Normal
			case DownMsg:
				// if the process we want to send a message to dies, cancel the timer by exiting
				if t.ref == msg.Ref {
					DebugPrintf("<timer:%v> process down, cancelling", self)
					return exitreason.Normal
				} else {
					DebugPrintf("<timer:%v> got unknown DownMsg: %+v", msg)
				}
			default:
				DebugPrintf("<timer:%v> received unknown message %+v", self, msg)
			}

		case <-time.After(t.tout):
			DebugPrintf("<timer:%v> firing msg %+v to %+v", self, t.term, t.to)
			Send(t.to, t.term)
			return exitreason.Normal
		}
	}
}
