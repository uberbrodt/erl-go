package application

import (
	"errors"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"

	"github.com/uberbrodt/erl-go/chronos"
	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/genserver"
	"github.com/uberbrodt/erl-go/erl/supervisor"
)

type testApp struct {
	startRet func(self erl.PID, args any) (erl.PID, error)
	stopRet  func() error
}

func (ta *testApp) Start(self erl.PID, args any) (erl.PID, error) {
	return ta.startRet(self, args)
}

func (ta *testApp) Stop() error {
	return ta.stopRet()
}

// var basicTestSrv =

func basicTestSrv(id string) supervisor.ChildSpec {
	return supervisor.NewTestServerChildSpec(id, genserver.NewTestServer[int](), genserver.DefaultOpts())
}

func TestStart_NoErrors(t *testing.T) {
	startRet := func(self erl.PID, args any) (erl.PID, error) {
		assert.Check(t, cmp.Equal(args, "hello"))
		children := []supervisor.ChildSpec{
			basicTestSrv("child1"),
		}
		return supervisor.StartDefaultLink(self, children, supervisor.NewSupFlags())
	}
	ta := &testApp{
		startRet: startRet,
		stopRet:  func() error { return nil },
	}

	app := Start(ta, "hello", func() {
		t.Logf("called cancel()")
	})
	assert.Assert(t, erl.IsAlive(app.self))
	assert.Assert(t, !app.Stopped())
}

func TestStart_ErrorsCausePanic(t *testing.T) {
	failSrv := genserver.NewTestServer[int](genserver.SetInitProbe[int](func(self erl.PID, args any) (int, any, error) {
		return 0, nil, errors.New("uh-oh")
	}))

	startRet := func(self erl.PID, args any) (erl.PID, error) {
		assert.Check(t, cmp.Equal(args, "hello"))
		children := []supervisor.ChildSpec{
			basicTestSrv("child1"),
			supervisor.NewTestServerChildSpec("child2", failSrv, genserver.DefaultOpts()),
		}
		return supervisor.StartDefaultLink(self, children, supervisor.NewSupFlags())
	}

	ta := &testApp{
		startRet: startRet,
		stopRet:  func() error { return nil },
	}

	var app *App
	assert.Check(t, cmp.Panics(func() {
		app = Start(ta, "hello", func() {
			t.Logf("called cancel()")
		})
	}))

	assert.Assert(t, app == nil)
}

type startTicking struct{}

func TestApp_SupervisorExitCallsCancel(t *testing.T) {
	timeBomb := genserver.NewTestServer[int](genserver.SetInitProbe[int](func(self erl.PID, args any) (int, any, error) {
		erl.Send(self, genserver.NewTestMsg[int](
			genserver.SetProbe[int](func(self erl.PID, arg any, state int) (any, int, error) {
				<-time.After(chronos.Dur("1s"))
				return nil, 2, errors.New("uh-oh")
			})))

		return 0, nil, nil
	}))

	startRet := func(self erl.PID, args any) (erl.PID, error) {
		assert.Check(t, cmp.Equal(args, "hello"))
		children := []supervisor.ChildSpec{
			supervisor.NewTestServerChildSpec("child1", timeBomb, genserver.DefaultOpts()),
			supervisor.NewTestServerChildSpec("child2", timeBomb, genserver.DefaultOpts()),
		}
		return supervisor.StartDefaultLink(self, children, supervisor.NewSupFlags(supervisor.SetIntensity(1)))
	}

	cancelled := make(chan string)
	ta := &testApp{
		startRet: startRet,
		stopRet:  func() error { return nil },
	}
	app := Start(ta, "hello", func() {
		cancelled <- "cancelled"
	})

	<-cancelled

	assert.Assert(t, app.Stopped())
	assert.Assert(t, !erl.IsAlive(app.self))
}

func TestApp_StopCallsCancel(t *testing.T) {
	startRet := func(self erl.PID, args any) (erl.PID, error) {
		assert.Check(t, cmp.Equal(args, "hello"))
		children := []supervisor.ChildSpec{
			basicTestSrv("child1"),
			basicTestSrv("child2"),
		}
		return supervisor.StartDefaultLink(self, children, supervisor.NewSupFlags(supervisor.SetIntensity(3)))
	}

	cancelled := make(chan string, 1)
	ta := &testApp{
		startRet: startRet,
		stopRet:  func() error { return nil },
	}
	app := Start(ta, "hello", func() {
		cancelled <- "cancelled"
	})

	err := app.Stop()

	<-cancelled

	assert.NilError(t, err)
	assert.Assert(t, app.Stopped())
	assert.Assert(t, !erl.IsAlive(app.self))
}
