package erl_test

import (
	"fmt"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/erltest"
	"github.com/uberbrodt/erl-go/erl/erltest/check"
	"github.com/uberbrodt/erl-go/erl/erltest/expect"
	"github.com/uberbrodt/erl-go/erl/erltest/testcase"
	"github.com/uberbrodt/erl-go/erl/exitreason"
)

func TestRegistration_ReRegisterNameAfterDownMsg(t *testing.T) {
	name := erl.Name("my_pid")
	tc := testcase.New(t, erltest.WaitTimeout(5*time.Second))

	var pid erl.PID

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

func TestRegistration_InvalidNames(t *testing.T) {
	name1 := erl.Name("")
	name2 := erl.Name("undefined")
	name3 := erl.Name("nil")

	var result1, result2, result3 *erl.RegistrationError

	tc := testcase.New(t, erltest.WaitTimeout(5*time.Second))

	tc.Arrange(func(self erl.PID) {
	})

	tc.Act(func() {
		result1 = erl.Register(name1, tc.TestPID())
		result2 = erl.Register(name2, tc.TestPID())
		result3 = erl.Register(name3, tc.TestPID())
	})

	tc.Assert(func() {
		check.Assert(t, result1.Kind == erl.BadName)
		check.Assert(t, result2.Kind == erl.BadName)
		check.Assert(t, result3.Kind == erl.BadName)
	})
}

func TestRegistration_NameInUse(t *testing.T) {
	var result *erl.RegistrationError
	var otherPID erl.PID

	tc := testcase.New(t, erltest.WaitTimeout(5*time.Second))

	tc.Arrange(func(self erl.PID) {
		otherPID, _ = erltest.NewReceiver(t)
		result = erl.Register(erl.Name("my_pid"), tc.TestPID())
	})

	tc.Act(func() {
		result = erl.Register(erl.Name("my_pid"), otherPID)
	})

	tc.Assert(func() {
		assert.Equal(t, result.Kind, erl.NameInUse)
	})
}

func TestRegistration_AlreadyRegistered(t *testing.T) {
	var result *erl.RegistrationError
	name := erl.Name("my_pid")
	otherName := erl.Name("my_pid")

	tc := testcase.New(t, erltest.WaitTimeout(5*time.Second))

	tc.Arrange(func(self erl.PID) {
		err := erl.Register(name, tc.TestPID())
		t.Logf("registration err: %v", err)
		assert.Assert(t, err == nil)
	})

	tc.Act(func() {
		result = erl.Register(otherName, tc.TestPID())
	})

	tc.Assert(func() {
		assert.Equal(t, result.Kind, erl.AlreadyRegistered)
	})
}

func TestRegistration_BadPidReturnsNoProc(t *testing.T) {
	var result *erl.RegistrationError

	tc := testcase.New(t, erltest.WaitTimeout(5*time.Second))

	tc.Arrange(func(self erl.PID) {
	})

	tc.Act(func() {
		result = erl.Register(erl.Name("my_pid"), erl.PID{})
	})

	tc.Assert(func() {
		assert.Assert(t, result.Kind == erl.NoProc)
	})
}

func TestWhereIs_NameFound(t *testing.T) {
	var found bool
	var foundPID erl.PID
	name := erl.Name("my_pid")

	tc := testcase.New(t, erltest.WaitTimeout(5*time.Second))

	tc.Arrange(func(self erl.PID) {
		err := erl.Register(name, tc.TestPID())
		assert.Assert(t, err == nil)
	})

	tc.Act(func() {
		foundPID, found = erl.WhereIs(name)
	})

	tc.Assert(func() {
		assert.Assert(t, found)
		assert.Equal(t, tc.TestPID(), foundPID)
	})
}

func TestWhereIs_NameNotFound(t *testing.T) {
	var found bool
	var foundPID erl.PID
	name := erl.Name("my_pid")
	badName := erl.Name("foo")

	tc := testcase.New(t, erltest.WaitTimeout(5*time.Second))

	tc.Arrange(func(self erl.PID) {
		err := erl.Register(name, tc.TestPID())
		assert.Assert(t, err == nil)
	})

	tc.Act(func() {
		foundPID, found = erl.WhereIs(badName)
	})

	tc.Assert(func() {
		assert.Assert(t, !found)
		assert.Assert(t, foundPID.IsNil())
	})
}

func TestRegistration_ReRegisterNameAfterExitMsg(t *testing.T) {
	name := erl.Name("my_pid")
	tc := testcase.New(t, erltest.WaitTimeout(5*time.Second))

	var pid erl.PID

	tc.Arrange(func(self erl.PID) {
		// pid = tc.Spawn(erltest.NewReceiver(t))
		pid, _ = erltest.NewReceiver(t)
		err := erl.Register(name, pid)

		assert.Assert(t, err == nil)
		erl.Link(self, pid)

		tc.Receiver().Expect(erl.ExitMsg{}, expect.Called(expect.Times(1)))
	})

	tc.Act(func() {
		erl.Exit(tc.TestPID(), pid, exitreason.TestExit)
	})

	tc.Assert(func() {
		_, exists := erl.WhereIs(name)
		assert.Assert(t, !exists)
		pid2, _ := erltest.NewReceiver(t)
		err := erl.Register(name, pid2)
		t.Logf("registration error: %+v", err)
		assert.Assert(t, err == nil)
	})
}

func TestRegistration_MassRegistration(t *testing.T) {
	t.Skip()
	for i := 0; i < 10; i++ {
		tc := testcase.New(t, erltest.WaitTimeout(500*time.Millisecond))
		name := erl.Name(fmt.Sprintf("my_pid-%d", i))

		var pid erl.PID

		tc.Arrange(func(self erl.PID) {
			// pid = tc.Spawn(erltest.NewReceiver(t))
			pid, _ = erltest.NewReceiver(t)
			err := erl.Register(name, pid)

			assert.Assert(t, err == nil)
			_ = erl.Monitor(self, pid)

			tc.Receiver().Expect(erl.DownMsg{}, expect.Called(expect.AtLeast(1)))
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
}
