package recurringtask

import (
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/uberbrodt/erl-go/chronos"
	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
	"github.com/uberbrodt/erl-go/erl/genserver"
)

type testState struct {
	t     *testing.T
	trPID erl.PID
}

type testTaskArgs struct {
	t     *testing.T
	trPID erl.PID
}
type taskRan struct{}

func testTaskFun(self erl.PID, state testState) (testState, error) {
	state.t.Logf("Running Task: %s", time.Now())
	erl.Send(state.trPID, taskRan{})
	return state, nil
}

func testInitFun(self erl.PID, args testTaskArgs) (testState, error) {
	args.t.Logf("initializing task")

	state := testState{t: args.t, trPID: args.trPID}

	return state, nil
}

func TestStartLink_Success(t *testing.T) {
	trPID, tr := erl.NewTestReceiver(t)
	pid, err := StartLink(trPID, testTaskFun, testInitFun, testTaskArgs{t: t, trPID: trPID}, SetInterval(chronos.Dur("1s")))

	assert.NilError(t, err)

	iterations := 0

	success := tr.Loop(func(anymsg any) bool {
		switch anymsg.(type) {
		case taskRan:
			iterations++
		}

		return iterations >= 2
	})

	assert.Assert(t, success)

	err = Stop(trPID, pid, genserver.StopReason(exitreason.SupervisorShutdown))
	assert.NilError(t, err)
}
