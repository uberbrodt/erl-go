package erl

import (
	"errors"
	"testing"
	"time"

	"github.com/uberbrodt/erl-go/chronos"
	"github.com/uberbrodt/erl-go/erl/exitreason"
)

var testTimeout time.Duration = chronos.Dur("10s")

// Deprecated: use [erltest.NewReceiver] as an alternative with the ability
// to set message expectations
func NewTestReceiver(t *testing.T) (PID, *TestReceiver) {
	c := make(chan any, 50)
	tr := &TestReceiver{c: c, t: t}
	pid := Spawn(tr)

	ProcessFlag(pid, TrapExit, true)
	t.Cleanup(func() {
		Exit(RootPID(), pid, exitreason.TestExit)
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
			switch v := msg.(type) {
			case ExitMsg:
				if errors.Is(v.Reason, exitreason.TestExit) {
					// NOTE: don't log exitmsg, it will cause a panic
					return exitreason.Normal
				}
				// default:
			}
			tr.t.Logf("TestReceiver got message: %#v", msg)
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

// Like [Loop] but exit after [tout]
func (tr *TestReceiver) LoopFor(tout time.Duration, handler func(msg any) bool) error {
	tr.t.Logf("TestReceiver starting Loop")
	for {
		select {
		case msg, ok := <-tr.c:
			tr.t.Logf("Loop got message: %#v", msg)
			if !ok {
				return nil
			}
			if stop := handler(msg); stop {
				return nil
			}

		case <-time.After(tout):

			return exitreason.Timeout
		}
	}
}

func (tr *TestReceiver) Loop(handler func(msg any) bool) bool {
	tr.t.Logf("TestReceiver starting Loop")
	for {
		select {
		case msg, ok := <-tr.c:
			tr.t.Logf("Loop got message: %#v", msg)
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
