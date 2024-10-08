package erl_test

import (
	"testing"
	"time"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/erltest"
	"github.com/uberbrodt/erl-go/erl/erltest/expect"
	"github.com/uberbrodt/erl-go/erl/erltest/testcase"
	"github.com/uberbrodt/erl-go/erl/exitreason"
	"gotest.tools/v3/assert"
)

func TestRegistration_ReRegisterName(t *testing.T) {
	name := erl.Name("my_pid")
	tc := testcase.New(t, erltest.WaitTimeout(5*time.Second))

	var pid erl.PID
	// var ref erl.Ref

	tc.Arrange(func(self erl.PID) {
		// pid = tc.Spawn(erltest.NewReceiver(t))
		pid, _ = erltest.NewReceiver(t)
		err := erl.Register(name, pid)

		assert.Assert(t, err == nil)
		_ = erl.Monitor(self, pid)

		tc.Receiver().Expect(erl.DownMsg{}, expect.Called(expect.Times(1)))
	})

	tc.Act(func() {
		erl.Exit(tc.TestPID(), pid, exitreason.TestExit)
	})

	tc.Assert(func() {
		pid2, _ := erltest.NewReceiver(t)
		err := erl.Register(name, pid2)
		t.Logf("registration error: %+v", err)
		assert.Assert(t, err == nil)
	})
}
