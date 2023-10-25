package erl

import (
	"testing"
	"time"

	"github.com/uberbrodt/erl-go/chronos"
	"github.com/uberbrodt/erl-go/erl/exitreason"
)

var testTimeout time.Duration = chronos.Dur("10s")

func NewTestReceiver(t *testing.T) (PID, *TestReceiver) {
	c := make(chan any, 50)
	tr := &TestReceiver{c: c, t: t}
	pid := Spawn(tr)

	ProcessFlag(pid, TrapExit, true)
	t.Cleanup(func() {
		Exit(RootPID(), pid, exitreason.Kill)
	})
	return pid, tr
}

type TestReceiver struct {
	c chan any
	t *testing.T
}

func (tr *TestReceiver) Receive(self PID, inbox <-chan any) error {
	for {
		select {
		case msg, ok := <-inbox:
			if !ok {
				return exitreason.Normal
			}
			tr.t.Logf("TestReceiver got message: %+v", msg)
			tr.c <- msg
		case <-time.After(testTimeout):
			tr.t.Fatal("TestReceiver: test timeout")

			return exitreason.Timeout
		}
	}
}

func (tr *TestReceiver) Receiver() <-chan any {
	return tr.c
}

func (tr *TestReceiver) Loop(handler func(msg any) bool) bool {
	tr.t.Logf("TestReceiver starting Loop")
	for {
		select {
		case msg, ok := <-tr.c:
			tr.t.Logf("Loop got message: %+v", msg)
			if !ok {
				return false
			}
			if stop := handler(msg); stop {
				return true
			}

		case <-time.After(testTimeout):
			tr.t.Fatal("TestReceiver.Loop test timeout")
			// close(th.c)

			return false
		}
	}
}
