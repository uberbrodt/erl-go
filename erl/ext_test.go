package erl_test

import (
	"testing"
	"time"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/erltest"
	"github.com/uberbrodt/erl-go/erl/erltest/check"
	"github.com/uberbrodt/erl-go/erl/erltest/expect"
	"github.com/uberbrodt/erl-go/erl/erltest/testcase"
)

type TestMsg struct {
	Name string
}

func TestExtMessageOrdering(t *testing.T) {
	tc := testcase.New(t, erltest.WaitTimeout(5*time.Second))

	expected := []TestMsg{
		{Name: "Blue"},
		{Name: "Green"},
		{Name: "Red"},
		{Name: "Orange"},
		{Name: "Black"},
		{Name: "Purple"},
		{Name: "Yellow"},
		{Name: "Chartruse"},
	}

	expectMsgs := func(msgs []TestMsg) *expect.Expectation {
		ex := expect.New(func(arg erltest.ExpectArg) (erltest.Expectation, *erltest.ExpectationFailure) {
			head, tail := msgs[0], msgs[1:]
			msgs = tail
			if ok := check.DeepEqual(t, head, arg.Msg); !ok {
				return nil, erltest.Fail(arg, "Msgs don't match")
			}
			return nil, nil
		}, expect.AtLeast(len(msgs)))
		return ex
	}

	tc.Arrange(func(self erl.PID) {
		tc.Receiver().Expect(TestMsg{}, expectMsgs(expected))
	})

	tc.Act(func() {
		erl.Send(tc.TestPID(), TestMsg{Name: "Blue"})
		erl.Send(tc.TestPID(), TestMsg{Name: "Green"})
		erl.Send(tc.TestPID(), TestMsg{Name: "Red"})
		erl.Send(tc.TestPID(), TestMsg{Name: "Orange"})
		erl.Send(tc.TestPID(), TestMsg{Name: "Black"})
		erl.Send(tc.TestPID(), TestMsg{Name: "Purple"})
		erl.Send(tc.TestPID(), TestMsg{Name: "Yellow"})
		erl.Send(tc.TestPID(), TestMsg{Name: "Chartruse"})
	})

	tc.Assert(func() {
	})
}
