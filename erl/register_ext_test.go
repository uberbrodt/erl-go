package erl_test

import (
	"fmt"
	"log/slog"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
	"github.com/uberbrodt/erl-go/erl/internal/test"
	"github.com/uberbrodt/erl-go/erl/x/erltest"
	"github.com/uberbrodt/erl-go/erl/x/erltest/check"
	"github.com/uberbrodt/erl-go/erl/x/erltest/testcase"
)

type NamedProcess struct{}

func (np NamedProcess) Receive(self erl.PID, inbox <-chan any) error {
	for msg := range inbox {
		slog.Info("got message", "msg", msg)
	}
	return nil
}

func TestRegistration_ReRegisterNameAfterDownMsg(t *testing.T) {
	name := erl.Name("49e71d92-bf7d-4ce2-8eca-10bafa7a09e8")
	tc := testcase.New(t, erltest.WaitTimeout(5*time.Second))

	var pid erl.PID

	tc.Arrange(func(self erl.PID) {
		// pid = tc.Spawn(erltest.NewReceiver(t))
		pid, _ = erltest.NewReceiver(t)
		err := erl.Register(name, pid)

		assert.Assert(t, err == nil)
		_ = erl.Monitor(self, pid)

		tc.Receiver().Expect(erl.DownMsg{}, gomock.Any())
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
	name := erl.Name("7782cdab-b213-4a4d-822c-02b2c235b629")

	tc := testcase.New(t, erltest.WaitTimeout(5*time.Second))

	tc.Arrange(func(self erl.PID) {
		otherPID, _ = erltest.NewReceiver(t)
		result = erl.Register(name, tc.TestPID())
	})

	tc.Act(func() {
		result = erl.Register(name, otherPID)
	})

	tc.Assert(func() {
		assert.Equal(t, result.Kind, erl.NameInUse)
	})
}

func TestRegistration_AlreadyRegistered(t *testing.T) {
	var result *erl.RegistrationError
	name := erl.Name("ae081f4d-4022-42aa-8371-0b211f00c9d4")
	otherName := erl.Name("d8683df9-1e57-4692-8ad1-22e42a8bead5")

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
	name := erl.Name("30d43088-290f-4b5b-a89e-4fc07877e786")

	tc := testcase.New(t, erltest.WaitTimeout(5*time.Second))

	tc.Arrange(func(self erl.PID) {
	})

	tc.Act(func() {
		result = erl.Register(name, erl.PID{})
	})

	tc.Assert(func() {
		assert.Assert(t, result.Kind == erl.NoProc)
	})
}

func TestWhereIs_NameFound(t *testing.T) {
	var found bool
	var foundPID erl.PID
	name := erl.Name("08cf9e7e-440e-4bf1-b805-0dbfbfa4c823")

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
	name := erl.Name("3b0da840-bb5d-44c3-b5f9-68375c81c41a")
	badName := erl.Name("61e3f530-e138-438c-8b91-2b76ecec62cc")

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
	name := erl.Name("7c5e8d3e-1bcb-44c3-bc48-e87a4b96196e")
	tc := testcase.New(t, erltest.WaitTimeout(5*time.Second))

	var pid erl.PID

	tc.Arrange(func(self erl.PID) {
		// pid = tc.Spawn(erltest.NewReceiver(t))
		pid, _ = erltest.NewReceiver(t)
		err := erl.Register(name, pid)

		assert.Assert(t, err == nil)
		erl.Link(self, pid)

		tc.Receiver().Expect(erl.ExitMsg{}, gomock.Any()).Times(1)
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
	test.SlowTest(t)
	for i := 0; i < 20; i++ {
		tc := testcase.New(t, erltest.WaitTimeout(30*time.Second))
		name := erl.Name(fmt.Sprintf("my_pid-%d", i))

		var pid erl.PID

		tc.Arrange(func(self erl.PID) {
			// pid = tc.Spawn(erltest.NewReceiver(t))
			pid = erl.Spawn(NamedProcess{})
			err := erl.Register(name, pid)

			assert.Assert(t, err == nil)
			_ = erl.Monitor(self, pid)

			tc.Receiver().Expect(erl.DownMsg{}, gomock.Any()).Times(1)
		})

		tc.Act(func() {
			erl.Exit(tc.TestPID(), pid, exitreason.Normal)
		})

		tc.Assert(func() {
			pid2 := erl.Spawn(NamedProcess{})
			err := erl.Register(name, pid2)
			t.Logf("registration error: %+v", err)
			assert.Assert(t, err == nil)
		})
	}
}
