package genserver

import (
	"errors"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/uberbrodt/erl-go/chronos"
	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
)

func TestStartLink_InitNoErrors(t *testing.T) {
	args := TestGSArgs{initProbe: func(self erl.PID, args any) (state TestGS, cont any, err error) {
		return TestGS{}, nil, nil
	}}

	gensrvPID, err := StartLink[TestGS](erl.RootPID(), TestGS{}, args)

	assert.NilError(t, err)
	assert.Assert(t, erl.IsAlive(gensrvPID))
}

func TestStartLink_InitExitExceptionStop(t *testing.T) {
	tr := &TestReceiver{
		c: make(chan any, 500),
		t: t,
	}

	trPID := erl.Spawn(tr)

	erl.ProcessFlag(trPID, erl.TrapExit, true)

	myErr := errors.New("Great, an error")
	args := TestGSArgs{initProbe: func(self erl.PID, args any) (state TestGS, cont any, err error) {
		return TestGS{}, nil, myErr
	}}

	gensrvPID, err := StartLink[TestGS](trPID, TestGS{}, args)

	var expectedErr *exitreason.S

	assert.Assert(t, errors.As(err, &expectedErr))
	assert.ErrorContains(t, expectedErr, "Great, an error")
	assert.Assert(t, exitreason.IsException(err))

	var success bool

	for msg := range tr.c {
		switch msgT := msg.(type) {
		case erl.ExitMsg:
			success = true

			assert.Assert(t, exitreason.IsException(msgT.Reason))
			assert.Assert(t, !erl.IsAlive(gensrvPID))

		default:
			t.Logf("test receiver got type[%T]: %+v", msg, msg)
		}
		if success {
			break
		}
	}

	assert.Assert(t, success)
}

func TestStartLink_InitIgnoreStop(t *testing.T) {
	// TODO: fix flakey test (or find the bug?)
	tr := &TestReceiver{
		c: make(chan any, 500),
		t: t,
	}

	trPID := erl.Spawn(tr)

	erl.ProcessFlag(trPID, erl.TrapExit, true)

	args := TestGSArgs{initProbe: func(self erl.PID, args any) (state TestGS, cont any, err error) {
		return TestGS{}, nil, exitreason.Ignore
	}}

	gensrvPID, err := StartLink[TestGS](trPID, TestGS{}, args)

	assert.ErrorIs(t, err, exitreason.Ignore)
	// TODO: since the process exits with exitreason.Normal, we won't get it's exit signal
	// so this is the best test we can do without instrumenting the GenServerS

	var success bool

	for msg := range tr.c {
		switch msgT := msg.(type) {
		case erl.ExitMsg:
			success = true
			assert.ErrorIs(t, msgT.Reason, exitreason.Normal)
			assert.Assert(t, !erl.IsAlive(gensrvPID))
		default:
			t.Logf("test receiver got type[%T]: %+v", msg, msg)
		}
		if success {
			break
		}
	}

	assert.Assert(t, success)
}

func TestStop_Defaults(t *testing.T) {
	args := TestGSArgs{initProbe: func(self erl.PID, args any) (state TestGS, cont any, err error) {
		return TestGS{}, nil, nil
	}}

	gensrvPID, err := StartLink[TestGS](erl.RootPID(), TestGS{}, args)

	assert.NilError(t, err)
	assert.Assert(t, erl.IsAlive(gensrvPID))

	result := Stop(erl.RootPID(), gensrvPID)

	assert.NilError(t, result)
	assert.Assert(t, !erl.IsAlive(gensrvPID))
}

func TestStop_ReturnsNoProcIfNameUnregistered(t *testing.T) {
	result := Stop(erl.RootPID(), erl.Name("asdkljasdkl;fj"))

	assert.ErrorIs(t, result, exitreason.NoProc)
}

func TestStop_ReturnsTimeoutIfExceeded(t *testing.T) {
	cb := TestGS{terminateProbe: func(self erl.PID, arg error, state TestGS) {
		erl.Logger.Println("terminateProbe called, waiting for 10s")
		<-time.After(chronos.Dur("10s"))
	}}
	gensrvPID, _ := StartLink[TestGS](erl.RootPID(), cb, TestGSArgs{})
	erl.ProcessFlag(gensrvPID, erl.TrapExit, true)

	result := Stop(erl.RootPID(), gensrvPID, StopTimeout(chronos.Dur("1s")))

	assert.ErrorIs(t, result, exitreason.Timeout)
}

func TestCast_ReturnsOKIfProcessExists(t *testing.T) {
	pid, err := startTestGS(erl.RootPID(), t, TestGS{}, TestGSArgs{})
	receive := make(chan int, 1)
	assert.NilError(t, err)

	result := Cast(pid, taggedRequest{
		probe: func(self erl.PID, state TestGS) (newState TestGS) {
			receive <- 1
			return state
		},
	})
	select {
	case intMsg := <-receive:
		assert.Equal(t, intMsg, 1)

	case <-time.After(chronos.Dur("5s")):
		t.Fatalf("timed out waiting for probe response")

	}
	assert.NilError(t, result)
}

func TestCast_ReturnsErrorIfNameNotRegistered(t *testing.T) {
	err := Cast(erl.Name("imaginary_process"), nil)

	assert.ErrorIs(t, err, exitreason.NoProc)
}

func TestStartMonitor_InitNoErrors(t *testing.T) {
	args := TestGSArgs{initProbe: func(self erl.PID, args any) (state TestGS, cont any, err error) {
		return TestGS{}, nil, nil
	}}

	gensrvPID, ref, err := StartMonitor[TestGS](erl.RootPID(), TestGS{}, args)

	assert.NilError(t, err)
	assert.Assert(t, ref != erl.UndefinedRef)
	assert.Assert(t, erl.IsAlive(gensrvPID))
}
