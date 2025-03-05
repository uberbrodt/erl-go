package exitwaiter_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
	"github.com/uberbrodt/erl-go/erl/exitwaiter"
	"github.com/uberbrodt/erl-go/erl/x/erltest/testserver"
)

func TestWaitsOnProcessesNotTrappingExits(t *testing.T) {
	t.Logf("[%s] Test Started", time.Now().Format(time.RFC3339Nano))
	var wg sync.WaitGroup
	pid, err := testserver.Start(erl.RootPID(), testserver.NewConfig())

	assert.NilError(t, err)
	assert.Assert(t, erl.IsAlive(pid))
	wg.Add(1)
	t.Logf("[%s] Subject initialized", time.Now().Format(time.RFC3339Nano))

	_, err = exitwaiter.New(t, erl.RootPID(), pid, &wg)
	assert.NilError(t, err)
	t.Logf("[%s] exitaiter started", time.Now().Format(time.RFC3339Nano))
	erl.Exit(erl.RootPID(), pid, exitreason.Kill)
	t.Logf("[%s] kill signal sent to %+v, now calling wg.Wait()", time.Now().Format(time.RFC3339Nano), pid)
	wg.Wait()
	t.Logf("[%s] wg.Wait() passed", time.Now().Format(time.RFC3339Nano))
}

func TestWaitsOnProcessesTrappingExits(t *testing.T) {
	t.Logf("[%s] Test Started", time.Now().Format(time.RFC3339Nano))
	var wg sync.WaitGroup
	pid, err := testserver.Start(erl.RootPID(), testserver.NewConfig().SetInit(InitTrapExit))

	assert.NilError(t, err)
	assert.Assert(t, erl.IsAlive(pid))
	wg.Add(1)
	t.Logf("[%s] Subject initialized", time.Now().Format(time.RFC3339Nano))

	_, err = exitwaiter.New(t, erl.RootPID(), pid, &wg)
	assert.NilError(t, err)
	t.Logf("[%s] exitaiter started", time.Now().Format(time.RFC3339Nano))
	erl.Exit(erl.RootPID(), pid, exitreason.Kill)
	t.Logf("[%s] kill signal sent to %+v, now calling wg.Wait()", time.Now().Format(time.RFC3339Nano), pid)
	wg.Wait()
	t.Logf("[%s] wg.Wait() passed", time.Now().Format(time.RFC3339Nano))
}

func TestClosesWaitGroupIfProcessAlreadyDead(t *testing.T) {
	t.Logf("[%s] Test Started", time.Now().Format(time.RFC3339Nano))
	var wg sync.WaitGroup
	pid, err := testserver.Start(erl.RootPID(), testserver.NewConfig().SetInit(InitTrapExit))

	assert.NilError(t, err)
	assert.Assert(t, erl.IsAlive(pid))
	wg.Add(1)
	t.Logf("[%s] Subject initialized", time.Now().Format(time.RFC3339Nano))

	erl.Exit(erl.RootPID(), pid, exitreason.Kill)
	t.Logf("[%s] kill signal sent to %+v", time.Now().Format(time.RFC3339Nano), pid)

	_, err = exitwaiter.New(t, erl.RootPID(), pid, &wg)
	assert.NilError(t, err)
	t.Logf("[%s] exitaiter started", time.Now().Format(time.RFC3339Nano))
	wg.Wait()
	t.Logf("[%s] wg.Wait() passed", time.Now().Format(time.RFC3339Nano))
}

func Test_WaitOnMultipleProcesses(t *testing.T) {
	t.Logf("[%s] Test Started", time.Now().Format(time.RFC3339Nano))
	var wg sync.WaitGroup
	pids := make([]erl.PID, 0, 10)
	for range 40 {
		pid, err := testserver.Start(erl.RootPID(), testserver.NewConfig().SetInit(testserver.InitOK))

		assert.NilError(t, err)
		assert.Assert(t, erl.IsAlive(pid))
		wg.Add(1)
		pids = append(pids, pid)
	}
	t.Logf("[%s] Subject initialized", time.Now().Format(time.RFC3339Nano))

	for _, pid := range pids {
		_, err := exitwaiter.New(t, erl.RootPID(), pid, &wg)
		assert.NilError(t, err)
		t.Logf("[%s] exitaiter started", time.Now().Format(time.RFC3339Nano))
	}

	for _, pid := range pids {
		erl.Exit(erl.RootPID(), pid, exitreason.Kill)
		t.Logf("[%s] kill signal sent to %+v", time.Now().Format(time.RFC3339Nano), pid)
	}
	t.Logf("[%s] now calling wg.Wait()", time.Now().Format(time.RFC3339Nano))
	wg.Wait()
	t.Logf("[%s] wg.Wait() passed", time.Now().Format(time.RFC3339Nano))
}

func InitTrapExit(self erl.PID, args any) (testserver.TestServer, any, error) {
	conf, ok := args.(*testserver.Config)

	if !ok {
		return testserver.TestServer{}, nil, exitreason.Exception(errors.New("Init arg must be a {}"))
	}

	erl.ProcessFlag(self, erl.TrapExit, true)
	return testserver.TestServer{Conf: conf}, nil, nil
}
