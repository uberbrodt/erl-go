package supervisor_test

import (
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/uberbrodt/erl-go/chronos"
	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
	"github.com/uberbrodt/erl-go/erl/genserver"
	"github.com/uberbrodt/erl-go/erl/supervisor"
	"github.com/uberbrodt/erl-go/erl/x/erltest"
)

// =============================================================================
// Test Helpers
// =============================================================================

// childState is the state for test child GenServers.
type childState struct {
	id string
}

// childServer implements the GenServer interface for testing.
type childServer struct {
	trPID        erl.PID
	initialState childState
}

func (cs childServer) Init(self erl.PID, args any) (genserver.InitResult[childState], error) {
	erl.Send(cs.trPID, childInitMsg{id: cs.initialState.id, pid: self})
	return genserver.InitResult[childState]{State: cs.initialState}, nil
}

func (cs childServer) Terminate(self erl.PID, reason error, state childState) {
	erl.Send(cs.trPID, childTerminateMsg{id: state.id, pid: self, reason: reason})
}

func (cs childServer) HandleCall(self erl.PID, request any, from genserver.From, state childState) (genserver.CallResult[childState], error) {
	switch msg := request.(type) {
	case crashMsg:
		return genserver.CallResult[childState]{Msg: "crashing", State: state}, msg.reason
	default:
		return genserver.CallResult[childState]{Msg: request, State: state}, nil
	}
}

func (cs childServer) HandleCast(self erl.PID, request any, state childState) (genserver.CastResult[childState], error) {
	return genserver.CastResult[childState]{State: state}, nil
}

func (cs childServer) HandleInfo(self erl.PID, msg any, state childState) (genserver.InfoResult[childState], error) {
	return genserver.InfoResult[childState]{State: state}, nil
}

func (cs childServer) HandleContinue(self erl.PID, continuation any, state childState) (childState, any, error) {
	return state, nil, nil
}

// Message types for test coordination.
type childInitMsg struct {
	id  string
	pid erl.PID
}

type childTerminateMsg struct {
	id     string
	pid    erl.PID
	reason error
}

type crashMsg struct {
	reason error
}

// makeChildSpec creates a child spec for testing with the given ID and restart type.
func makeChildSpec(id string, name erl.Name, trPID erl.PID, restart supervisor.Restart) supervisor.ChildSpec {
	cs := childServer{trPID: trPID, initialState: childState{id: id}}
	return supervisor.NewChildSpec(id,
		func(sup erl.PID) (erl.PID, error) {
			return genserver.StartLink[childState](sup, cs, nil, genserver.SetName(name))
		},
		supervisor.SetRestart(restart),
	)
}

// startTestSupervisor starts a supervisor with the given flags and child specs.
func startTestSupervisor(t *testing.T, flags supervisor.SupFlagsS, childSpecs []supervisor.ChildSpec) erl.PID {
	t.Helper()
	supPID, err := supervisor.StartDefaultLink(
		erl.RootPID(),
		childSpecs,
		flags,
		supervisor.SetName("test-sup"),
	)
	assert.NilError(t, err)
	t.Cleanup(func() {
		if erl.IsAlive(supPID) {
			erl.Exit(erl.RootPID(), supPID, exitreason.Kill)
			time.Sleep(50 * time.Millisecond)
		}
	})
	return supPID
}

// =============================================================================
// OneForOne Strategy Tests
// =============================================================================

func TestOneForOne_OnlyFailedChildRestarts(t *testing.T) {
	trPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(5*time.Second))

	childSpecs := []supervisor.ChildSpec{
		makeChildSpec("child1", "child1-name", trPID, supervisor.Permanent),
		makeChildSpec("child2", "child2-name", trPID, supervisor.Permanent),
		makeChildSpec("child3", "child3-name", trPID, supervisor.Permanent),
	}

	flags := supervisor.NewSupFlags(
		supervisor.SetStrategy(supervisor.OneForOne),
		supervisor.SetIntensity(5),
		supervisor.SetPeriod(10),
	)

	// Track init messages to know when children are ready
	var initCount atomic.Int32
	tr.Expect(childInitMsg{}, gomock.Any()).AnyTimes().Do(func(arg erltest.ExpectArg) {
		initCount.Add(1)
	})
	tr.Expect(childTerminateMsg{}, gomock.Any()).AnyTimes()

	supPID := startTestSupervisor(t, flags, childSpecs)
	assert.Assert(t, erl.IsAlive(supPID))

	// Wait for all 3 children to init
	tr.WaitOnFunc(func() bool {
		return initCount.Load() >= 3
	})

	// Get PIDs of all children
	child1PID, ok := erl.WhereIs("child1-name")
	assert.Assert(t, ok)
	child2PID, ok := erl.WhereIs("child2-name")
	assert.Assert(t, ok)
	child3PID, ok := erl.WhereIs("child3-name")
	assert.Assert(t, ok)

	// Reset counter to track restart
	initCount.Store(0)

	// Crash child2
	erl.Exit(trPID, child2PID, exitreason.Kill)

	// Wait for child2 to reinit
	tr.WaitOnFunc(func() bool {
		return initCount.Load() >= 1
	})

	// Verify: child1 and child3 still have the same PIDs
	child1PIDAfter, ok := erl.WhereIs("child1-name")
	assert.Assert(t, ok)
	assert.Assert(t, child1PID.Equals(child1PIDAfter), "child1 should not have been restarted")

	child3PIDAfter, ok := erl.WhereIs("child3-name")
	assert.Assert(t, ok)
	assert.Assert(t, child3PID.Equals(child3PIDAfter), "child3 should not have been restarted")

	// Verify: child2 has a new PID (was restarted)
	child2PIDAfter, ok := erl.WhereIs("child2-name")
	assert.Assert(t, ok)
	assert.Assert(t, !child2PID.Equals(child2PIDAfter), "child2 should have been restarted with new PID")
}

func TestOneForOne_MultipleChildrenCrash_IndependentRestarts(t *testing.T) {
	trPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(5*time.Second))

	childSpecs := []supervisor.ChildSpec{
		makeChildSpec("child1", "child1-name", trPID, supervisor.Permanent),
		makeChildSpec("child2", "child2-name", trPID, supervisor.Permanent),
	}

	flags := supervisor.NewSupFlags(
		supervisor.SetStrategy(supervisor.OneForOne),
		supervisor.SetIntensity(5),
		supervisor.SetPeriod(10),
	)

	var initCount atomic.Int32
	tr.Expect(childInitMsg{}, gomock.Any()).AnyTimes().Do(func(arg erltest.ExpectArg) {
		initCount.Add(1)
	})
	tr.Expect(childTerminateMsg{}, gomock.Any()).AnyTimes()

	supPID := startTestSupervisor(t, flags, childSpecs)
	assert.Assert(t, erl.IsAlive(supPID))

	tr.WaitOnFunc(func() bool {
		return initCount.Load() >= 2
	})

	child1PID, _ := erl.WhereIs("child1-name")
	child2PID, _ := erl.WhereIs("child2-name")

	initCount.Store(0)

	// Crash both children
	erl.Exit(trPID, child1PID, exitreason.Kill)
	erl.Exit(trPID, child2PID, exitreason.Kill)

	// Wait for both to reinit
	tr.WaitOnFunc(func() bool {
		return initCount.Load() >= 2
	})

	// Both should have new PIDs
	child1PIDAfter, ok := erl.WhereIs("child1-name")
	assert.Assert(t, ok)
	assert.Assert(t, !child1PID.Equals(child1PIDAfter))

	child2PIDAfter, ok := erl.WhereIs("child2-name")
	assert.Assert(t, ok)
	assert.Assert(t, !child2PID.Equals(child2PIDAfter))
}

// =============================================================================
// OneForAll Strategy Tests
// =============================================================================

func TestOneForAll_AllChildrenRestartWhenOneFailsFirst(t *testing.T) {
	trPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(5*time.Second))

	childSpecs := []supervisor.ChildSpec{
		makeChildSpec("child1", "child1-name", trPID, supervisor.Permanent),
		makeChildSpec("child2", "child2-name", trPID, supervisor.Permanent),
		makeChildSpec("child3", "child3-name", trPID, supervisor.Permanent),
	}

	flags := supervisor.NewSupFlags(
		supervisor.SetStrategy(supervisor.OneForAll),
		supervisor.SetIntensity(5),
		supervisor.SetPeriod(10),
	)

	var initCount atomic.Int32
	tr.Expect(childInitMsg{}, gomock.Any()).AnyTimes().Do(func(arg erltest.ExpectArg) {
		initCount.Add(1)
	})
	tr.Expect(childTerminateMsg{}, gomock.Any()).AnyTimes()

	supPID := startTestSupervisor(t, flags, childSpecs)
	assert.Assert(t, erl.IsAlive(supPID))

	tr.WaitOnFunc(func() bool {
		return initCount.Load() >= 3
	})

	child1PID, _ := erl.WhereIs("child1-name")
	child2PID, _ := erl.WhereIs("child2-name")
	child3PID, _ := erl.WhereIs("child3-name")

	initCount.Store(0)

	// Crash the first child - all should restart
	erl.Exit(trPID, child1PID, exitreason.Kill)

	// Wait for all 3 to reinit
	tr.WaitOnFunc(func() bool {
		return initCount.Load() >= 3
	})

	// All children should have new PIDs
	child1PIDAfter, ok := erl.WhereIs("child1-name")
	assert.Assert(t, ok)
	assert.Assert(t, !child1PID.Equals(child1PIDAfter), "child1 should have new PID")

	child2PIDAfter, ok := erl.WhereIs("child2-name")
	assert.Assert(t, ok)
	assert.Assert(t, !child2PID.Equals(child2PIDAfter), "child2 should have new PID")

	child3PIDAfter, ok := erl.WhereIs("child3-name")
	assert.Assert(t, ok)
	assert.Assert(t, !child3PID.Equals(child3PIDAfter), "child3 should have new PID")
}

func TestOneForAll_AllChildrenRestartWhenMiddleChildFails(t *testing.T) {
	trPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(5*time.Second))

	childSpecs := []supervisor.ChildSpec{
		makeChildSpec("child1", "child1-name", trPID, supervisor.Permanent),
		makeChildSpec("child2", "child2-name", trPID, supervisor.Permanent),
		makeChildSpec("child3", "child3-name", trPID, supervisor.Permanent),
	}

	flags := supervisor.NewSupFlags(
		supervisor.SetStrategy(supervisor.OneForAll),
		supervisor.SetIntensity(5),
		supervisor.SetPeriod(10),
	)

	var initCount atomic.Int32
	tr.Expect(childInitMsg{}, gomock.Any()).AnyTimes().Do(func(arg erltest.ExpectArg) {
		initCount.Add(1)
	})
	tr.Expect(childTerminateMsg{}, gomock.Any()).AnyTimes()

	supPID := startTestSupervisor(t, flags, childSpecs)
	assert.Assert(t, erl.IsAlive(supPID))

	tr.WaitOnFunc(func() bool {
		return initCount.Load() >= 3
	})

	child1PID, _ := erl.WhereIs("child1-name")
	child2PID, _ := erl.WhereIs("child2-name")
	child3PID, _ := erl.WhereIs("child3-name")

	initCount.Store(0)

	// Crash middle child - all should restart
	erl.Exit(trPID, child2PID, exitreason.Kill)

	tr.WaitOnFunc(func() bool {
		return initCount.Load() >= 3
	})

	child1PIDAfter, _ := erl.WhereIs("child1-name")
	assert.Assert(t, !child1PID.Equals(child1PIDAfter))

	child2PIDAfter, _ := erl.WhereIs("child2-name")
	assert.Assert(t, !child2PID.Equals(child2PIDAfter))

	child3PIDAfter, _ := erl.WhereIs("child3-name")
	assert.Assert(t, !child3PID.Equals(child3PIDAfter))
}

func TestOneForAll_AllChildrenRestartWhenLastChildFails(t *testing.T) {
	trPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(5*time.Second))

	childSpecs := []supervisor.ChildSpec{
		makeChildSpec("child1", "child1-name", trPID, supervisor.Permanent),
		makeChildSpec("child2", "child2-name", trPID, supervisor.Permanent),
		makeChildSpec("child3", "child3-name", trPID, supervisor.Permanent),
	}

	flags := supervisor.NewSupFlags(
		supervisor.SetStrategy(supervisor.OneForAll),
		supervisor.SetIntensity(5),
		supervisor.SetPeriod(10),
	)

	var initCount atomic.Int32
	tr.Expect(childInitMsg{}, gomock.Any()).AnyTimes().Do(func(arg erltest.ExpectArg) {
		initCount.Add(1)
	})
	tr.Expect(childTerminateMsg{}, gomock.Any()).AnyTimes()

	supPID := startTestSupervisor(t, flags, childSpecs)
	assert.Assert(t, erl.IsAlive(supPID))

	tr.WaitOnFunc(func() bool {
		return initCount.Load() >= 3
	})

	child1PID, _ := erl.WhereIs("child1-name")
	child2PID, _ := erl.WhereIs("child2-name")
	child3PID, _ := erl.WhereIs("child3-name")

	initCount.Store(0)

	// Crash last child - all should restart
	erl.Exit(trPID, child3PID, exitreason.Kill)

	tr.WaitOnFunc(func() bool {
		return initCount.Load() >= 3
	})

	child1PIDAfter, _ := erl.WhereIs("child1-name")
	assert.Assert(t, !child1PID.Equals(child1PIDAfter))

	child2PIDAfter, _ := erl.WhereIs("child2-name")
	assert.Assert(t, !child2PID.Equals(child2PIDAfter))

	child3PIDAfter, _ := erl.WhereIs("child3-name")
	assert.Assert(t, !child3PID.Equals(child3PIDAfter))
}

// =============================================================================
// RestForOne Strategy Tests
// =============================================================================

func TestRestForOne_OnlyLaterChildrenRestartWhenMiddleChildFails(t *testing.T) {
	trPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(5*time.Second))

	childSpecs := []supervisor.ChildSpec{
		makeChildSpec("child1", "child1-name", trPID, supervisor.Permanent),
		makeChildSpec("child2", "child2-name", trPID, supervisor.Permanent),
		makeChildSpec("child3", "child3-name", trPID, supervisor.Permanent),
		makeChildSpec("child4", "child4-name", trPID, supervisor.Permanent),
	}

	flags := supervisor.NewSupFlags(
		supervisor.SetStrategy(supervisor.RestForOne),
		supervisor.SetIntensity(5),
		supervisor.SetPeriod(10),
	)

	var initCount atomic.Int32
	tr.Expect(childInitMsg{}, gomock.Any()).AnyTimes().Do(func(arg erltest.ExpectArg) {
		initCount.Add(1)
	})
	tr.Expect(childTerminateMsg{}, gomock.Any()).AnyTimes()

	supPID := startTestSupervisor(t, flags, childSpecs)
	assert.Assert(t, erl.IsAlive(supPID))

	tr.WaitOnFunc(func() bool {
		return initCount.Load() >= 4
	})

	child1PID, _ := erl.WhereIs("child1-name")
	child2PID, _ := erl.WhereIs("child2-name")
	child3PID, _ := erl.WhereIs("child3-name")
	child4PID, _ := erl.WhereIs("child4-name")

	initCount.Store(0)

	// Crash child2 - child2, child3, child4 should restart, child1 should NOT
	erl.Exit(trPID, child2PID, exitreason.Kill)

	// Wait for 3 reinits (child2, child3, child4)
	tr.WaitOnFunc(func() bool {
		return initCount.Load() >= 3
	})

	// child1 should have SAME PID (not restarted)
	child1PIDAfter, ok := erl.WhereIs("child1-name")
	assert.Assert(t, ok)
	assert.Assert(t, child1PID.Equals(child1PIDAfter), "child1 should NOT have been restarted")

	// child2, child3, child4 should have NEW PIDs
	child2PIDAfter, _ := erl.WhereIs("child2-name")
	assert.Assert(t, !child2PID.Equals(child2PIDAfter), "child2 should have new PID")

	child3PIDAfter, _ := erl.WhereIs("child3-name")
	assert.Assert(t, !child3PID.Equals(child3PIDAfter), "child3 should have new PID")

	child4PIDAfter, _ := erl.WhereIs("child4-name")
	assert.Assert(t, !child4PID.Equals(child4PIDAfter), "child4 should have new PID")
}

func TestRestForOne_AllRestartWhenFirstChildFails(t *testing.T) {
	trPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(5*time.Second))

	childSpecs := []supervisor.ChildSpec{
		makeChildSpec("child1", "child1-name", trPID, supervisor.Permanent),
		makeChildSpec("child2", "child2-name", trPID, supervisor.Permanent),
		makeChildSpec("child3", "child3-name", trPID, supervisor.Permanent),
	}

	flags := supervisor.NewSupFlags(
		supervisor.SetStrategy(supervisor.RestForOne),
		supervisor.SetIntensity(5),
		supervisor.SetPeriod(10),
	)

	var initCount atomic.Int32
	tr.Expect(childInitMsg{}, gomock.Any()).AnyTimes().Do(func(arg erltest.ExpectArg) {
		initCount.Add(1)
	})
	tr.Expect(childTerminateMsg{}, gomock.Any()).AnyTimes()

	supPID := startTestSupervisor(t, flags, childSpecs)
	assert.Assert(t, erl.IsAlive(supPID))

	tr.WaitOnFunc(func() bool {
		return initCount.Load() >= 3
	})

	child1PID, _ := erl.WhereIs("child1-name")
	child2PID, _ := erl.WhereIs("child2-name")
	child3PID, _ := erl.WhereIs("child3-name")

	initCount.Store(0)

	// Crash first child - all should restart (same as OneForAll for first child)
	erl.Exit(trPID, child1PID, exitreason.Kill)

	tr.WaitOnFunc(func() bool {
		return initCount.Load() >= 3
	})

	child1PIDAfter, _ := erl.WhereIs("child1-name")
	assert.Assert(t, !child1PID.Equals(child1PIDAfter))

	child2PIDAfter, _ := erl.WhereIs("child2-name")
	assert.Assert(t, !child2PID.Equals(child2PIDAfter))

	child3PIDAfter, _ := erl.WhereIs("child3-name")
	assert.Assert(t, !child3PID.Equals(child3PIDAfter))
}

func TestRestForOne_OnlyLastChildRestartsWhenLastChildFails(t *testing.T) {
	trPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(5*time.Second))

	childSpecs := []supervisor.ChildSpec{
		makeChildSpec("child1", "child1-name", trPID, supervisor.Permanent),
		makeChildSpec("child2", "child2-name", trPID, supervisor.Permanent),
		makeChildSpec("child3", "child3-name", trPID, supervisor.Permanent),
	}

	flags := supervisor.NewSupFlags(
		supervisor.SetStrategy(supervisor.RestForOne),
		supervisor.SetIntensity(5),
		supervisor.SetPeriod(10),
	)

	var initCount atomic.Int32
	tr.Expect(childInitMsg{}, gomock.Any()).AnyTimes().Do(func(arg erltest.ExpectArg) {
		initCount.Add(1)
	})
	tr.Expect(childTerminateMsg{}, gomock.Any()).AnyTimes()

	supPID := startTestSupervisor(t, flags, childSpecs)
	assert.Assert(t, erl.IsAlive(supPID))

	tr.WaitOnFunc(func() bool {
		return initCount.Load() >= 3
	})

	child1PID, _ := erl.WhereIs("child1-name")
	child2PID, _ := erl.WhereIs("child2-name")
	child3PID, _ := erl.WhereIs("child3-name")

	initCount.Store(0)

	// Crash last child - only it restarts (same as OneForOne for last child)
	erl.Exit(trPID, child3PID, exitreason.Kill)

	tr.WaitOnFunc(func() bool {
		return initCount.Load() >= 1
	})

	// child1 and child2 should have SAME PIDs
	child1PIDAfter, _ := erl.WhereIs("child1-name")
	assert.Assert(t, child1PID.Equals(child1PIDAfter), "child1 should NOT have been restarted")

	child2PIDAfter, _ := erl.WhereIs("child2-name")
	assert.Assert(t, child2PID.Equals(child2PIDAfter), "child2 should NOT have been restarted")

	// child3 should have NEW PID
	child3PIDAfter, _ := erl.WhereIs("child3-name")
	assert.Assert(t, !child3PID.Equals(child3PIDAfter), "child3 should have new PID")
}

// =============================================================================
// Restart Type Tests (Permanent, Transient, Temporary)
// =============================================================================

func TestPermanent_AlwaysRestarts(t *testing.T) {
	trPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(5*time.Second))

	childSpecs := []supervisor.ChildSpec{
		makeChildSpec("child1", "child1-name", trPID, supervisor.Permanent),
	}

	flags := supervisor.NewSupFlags(
		supervisor.SetStrategy(supervisor.OneForOne),
		supervisor.SetIntensity(5),
		supervisor.SetPeriod(10),
	)

	var initCount atomic.Int32
	tr.Expect(childInitMsg{}, gomock.Any()).AnyTimes().Do(func(arg erltest.ExpectArg) {
		initCount.Add(1)
	})
	tr.Expect(childTerminateMsg{}, gomock.Any()).AnyTimes()

	supPID := startTestSupervisor(t, flags, childSpecs)
	assert.Assert(t, erl.IsAlive(supPID))

	tr.WaitOnFunc(func() bool {
		return initCount.Load() >= 1
	})

	child1PID, _ := erl.WhereIs("child1-name")
	initCount.Store(0)

	// Permanent child should restart even on normal exit
	genserver.Call(trPID, child1PID, crashMsg{reason: exitreason.Normal}, chronos.Dur("5s"))

	tr.WaitOnFunc(func() bool {
		return initCount.Load() >= 1
	})

	child1PIDAfter, ok := erl.WhereIs("child1-name")
	assert.Assert(t, ok, "permanent child should have been restarted")
	assert.Assert(t, !child1PID.Equals(child1PIDAfter), "permanent child should have new PID")
}

func TestTransient_RestartsOnAbnormalExit(t *testing.T) {
	trPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(5*time.Second))

	childSpecs := []supervisor.ChildSpec{
		makeChildSpec("child1", "child1-name", trPID, supervisor.Transient),
	}

	flags := supervisor.NewSupFlags(
		supervisor.SetStrategy(supervisor.OneForOne),
		supervisor.SetIntensity(5),
		supervisor.SetPeriod(10),
	)

	var initCount atomic.Int32
	tr.Expect(childInitMsg{}, gomock.Any()).AnyTimes().Do(func(arg erltest.ExpectArg) {
		initCount.Add(1)
	})
	tr.Expect(childTerminateMsg{}, gomock.Any()).AnyTimes()

	supPID := startTestSupervisor(t, flags, childSpecs)
	assert.Assert(t, erl.IsAlive(supPID))

	tr.WaitOnFunc(func() bool {
		return initCount.Load() >= 1
	})

	child1PID, _ := erl.WhereIs("child1-name")
	initCount.Store(0)

	// Transient child should restart on abnormal exit (Kill)
	erl.Exit(trPID, child1PID, exitreason.Kill)

	tr.WaitOnFunc(func() bool {
		return initCount.Load() >= 1
	})

	child1PIDAfter, ok := erl.WhereIs("child1-name")
	assert.Assert(t, ok, "transient child should have been restarted after abnormal exit")
	assert.Assert(t, !child1PID.Equals(child1PIDAfter))
}

func TestTransient_DoesNotRestartOnNormalExit(t *testing.T) {
	trPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(3*time.Second))

	childSpecs := []supervisor.ChildSpec{
		makeChildSpec("child1", "child1-name", trPID, supervisor.Transient),
	}

	flags := supervisor.NewSupFlags(
		supervisor.SetStrategy(supervisor.OneForOne),
		supervisor.SetIntensity(5),
		supervisor.SetPeriod(10),
	)

	var initCount atomic.Int32
	var terminateCount atomic.Int32
	tr.Expect(childInitMsg{}, gomock.Any()).AnyTimes().Do(func(arg erltest.ExpectArg) {
		initCount.Add(1)
	})
	tr.Expect(childTerminateMsg{}, gomock.Any()).AnyTimes().Do(func(arg erltest.ExpectArg) {
		terminateCount.Add(1)
	})

	supPID := startTestSupervisor(t, flags, childSpecs)
	assert.Assert(t, erl.IsAlive(supPID))

	tr.WaitOnFunc(func() bool {
		return initCount.Load() >= 1
	})

	child1PID, _ := erl.WhereIs("child1-name")
	initCount.Store(0)

	// Cause normal exit
	genserver.Call(trPID, child1PID, crashMsg{reason: exitreason.Normal}, chronos.Dur("5s"))

	// Wait for terminate
	tr.WaitOnFunc(func() bool {
		return terminateCount.Load() >= 1
	})

	// Give time for any spurious restart
	time.Sleep(100 * time.Millisecond)

	// Transient should NOT restart on normal exit
	_, ok := erl.WhereIs("child1-name")
	assert.Assert(t, !ok, "transient child should NOT have been restarted after normal exit")
	assert.Assert(t, initCount.Load() == 0, "no init should have occurred after normal exit")

	// Supervisor should still be alive
	assert.Assert(t, erl.IsAlive(supPID))
}

func TestTransient_DoesNotRestartOnShutdownExit(t *testing.T) {
	trPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(3*time.Second))

	childSpecs := []supervisor.ChildSpec{
		makeChildSpec("child1", "child1-name", trPID, supervisor.Transient),
	}

	flags := supervisor.NewSupFlags(
		supervisor.SetStrategy(supervisor.OneForOne),
		supervisor.SetIntensity(5),
		supervisor.SetPeriod(10),
	)

	var initCount atomic.Int32
	var terminateCount atomic.Int32
	tr.Expect(childInitMsg{}, gomock.Any()).AnyTimes().Do(func(arg erltest.ExpectArg) {
		initCount.Add(1)
	})
	tr.Expect(childTerminateMsg{}, gomock.Any()).AnyTimes().Do(func(arg erltest.ExpectArg) {
		terminateCount.Add(1)
	})

	supPID := startTestSupervisor(t, flags, childSpecs)
	assert.Assert(t, erl.IsAlive(supPID))

	tr.WaitOnFunc(func() bool {
		return initCount.Load() >= 1
	})

	child1PID, _ := erl.WhereIs("child1-name")
	initCount.Store(0)

	genserver.Call(trPID, child1PID, crashMsg{reason: exitreason.Shutdown("planned shutdown")}, chronos.Dur("5s"))

	tr.WaitOnFunc(func() bool {
		return terminateCount.Load() >= 1
	})

	time.Sleep(100 * time.Millisecond)

	_, ok := erl.WhereIs("child1-name")
	assert.Assert(t, !ok, "transient child should NOT have been restarted after shutdown exit")
	assert.Assert(t, erl.IsAlive(supPID))
}

func TestTemporary_NeverRestarts(t *testing.T) {
	trPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(3*time.Second))

	childSpecs := []supervisor.ChildSpec{
		makeChildSpec("child1", "child1-name", trPID, supervisor.Temporary),
	}

	flags := supervisor.NewSupFlags(
		supervisor.SetStrategy(supervisor.OneForOne),
		supervisor.SetIntensity(5),
		supervisor.SetPeriod(10),
	)

	var initCount atomic.Int32
	tr.Expect(childInitMsg{}, gomock.Any()).AnyTimes().Do(func(arg erltest.ExpectArg) {
		initCount.Add(1)
	})
	// Note: exitreason.Kill bypasses Terminate callback, so we can't rely on terminateCount
	tr.Expect(childTerminateMsg{}, gomock.Any()).AnyTimes()

	supPID := startTestSupervisor(t, flags, childSpecs)
	assert.Assert(t, erl.IsAlive(supPID))

	tr.WaitOnFunc(func() bool {
		return initCount.Load() >= 1
	})

	child1PID, _ := erl.WhereIs("child1-name")
	initCount.Store(0)

	// Even abnormal exit should not restart temporary child
	// Note: exitreason.Kill bypasses Terminate callback (Erlang behavior)
	erl.Exit(trPID, child1PID, exitreason.Kill)

	// Wait for the process to be unregistered (killed)
	tr.WaitOnFunc(func() bool {
		_, ok := erl.WhereIs("child1-name")
		return !ok
	})

	// Give time for any spurious restart
	time.Sleep(100 * time.Millisecond)

	// Verify it wasn't restarted
	_, ok := erl.WhereIs("child1-name")
	assert.Assert(t, !ok, "temporary child should NEVER be restarted")
	assert.Assert(t, initCount.Load() == 0, "no init should have occurred after exit")
	assert.Assert(t, erl.IsAlive(supPID))
}

func TestTemporary_DoesNotCountTowardRestartIntensity(t *testing.T) {
	trPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(5*time.Second))

	childSpecs := []supervisor.ChildSpec{
		makeChildSpec("temp1", "temp1-name", trPID, supervisor.Temporary),
		makeChildSpec("temp2", "temp2-name", trPID, supervisor.Temporary),
		makeChildSpec("perm1", "perm1-name", trPID, supervisor.Permanent),
	}

	// Very low intensity - would crash if temporary exits counted
	flags := supervisor.NewSupFlags(
		supervisor.SetStrategy(supervisor.OneForOne),
		supervisor.SetIntensity(1),
		supervisor.SetPeriod(10),
	)

	var initCount atomic.Int32
	tr.Expect(childInitMsg{}, gomock.Any()).AnyTimes().Do(func(arg erltest.ExpectArg) {
		initCount.Add(1)
	})
	// Note: exitreason.Kill bypasses Terminate callback, so we can't rely on terminateCount
	tr.Expect(childTerminateMsg{}, gomock.Any()).AnyTimes()

	supPID := startTestSupervisor(t, flags, childSpecs)
	assert.Assert(t, erl.IsAlive(supPID))

	tr.WaitOnFunc(func() bool {
		return initCount.Load() >= 3
	})

	temp1PID, _ := erl.WhereIs("temp1-name")
	temp2PID, _ := erl.WhereIs("temp2-name")

	// Kill both temporary children - should not affect restart intensity
	// Note: exitreason.Kill bypasses Terminate callback (Erlang behavior)
	erl.Exit(trPID, temp1PID, exitreason.Kill)
	erl.Exit(trPID, temp2PID, exitreason.Kill)

	// Wait for both processes to be unregistered (killed)
	tr.WaitOnFunc(func() bool {
		_, ok1 := erl.WhereIs("temp1-name")
		_, ok2 := erl.WhereIs("temp2-name")
		return !ok1 && !ok2
	})

	// Give some time for any potential supervisor crash
	time.Sleep(100 * time.Millisecond)

	// Supervisor should still be alive despite 2 child exits
	assert.Assert(t, erl.IsAlive(supPID), "supervisor should still be alive - temporary exits don't count toward intensity")
}

// =============================================================================
// Restart Intensity Tests
// =============================================================================

func TestRestartIntensity_SupervisorCrashesWhenExceeded(t *testing.T) {
	trPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(5*time.Second))

	childSpecs := []supervisor.ChildSpec{
		makeChildSpec("child1", "child1-name", trPID, supervisor.Permanent),
	}

	// Allow only 1 restart in 10 seconds
	flags := supervisor.NewSupFlags(
		supervisor.SetStrategy(supervisor.OneForOne),
		supervisor.SetIntensity(1),
		supervisor.SetPeriod(10),
	)

	var initCount atomic.Int32
	tr.Expect(childInitMsg{}, gomock.Any()).AnyTimes().Do(func(arg erltest.ExpectArg) {
		initCount.Add(1)
	})
	tr.Expect(childTerminateMsg{}, gomock.Any()).AnyTimes()
	tr.Expect(erl.ExitMsg{}, gomock.Any()).AnyTimes()

	supPID := startTestSupervisor(t, flags, childSpecs)
	erl.Link(trPID, supPID)
	assert.Assert(t, erl.IsAlive(supPID))

	tr.WaitOnFunc(func() bool {
		return initCount.Load() >= 1
	})

	child1PID, _ := erl.WhereIs("child1-name")
	initCount.Store(0)

	// First restart should succeed
	erl.Exit(trPID, child1PID, exitreason.Kill)

	tr.WaitOnFunc(func() bool {
		return initCount.Load() >= 1
	})

	child1PIDAfter, _ := erl.WhereIs("child1-name")

	// Second crash should exceed intensity and crash supervisor
	erl.Exit(trPID, child1PIDAfter, exitreason.Kill)

	// Wait for supervisor to crash
	tr.WaitOnFunc(func() bool {
		return !erl.IsAlive(supPID)
	})

	assert.Assert(t, !erl.IsAlive(supPID), "supervisor should have crashed due to exceeded restart intensity")
}

func TestRestartIntensity_CounterResetsAfterPeriod(t *testing.T) {
	trPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(10*time.Second))

	childSpecs := []supervisor.ChildSpec{
		makeChildSpec("child1", "child1-name", trPID, supervisor.Permanent),
	}

	// Allow 1 restart per 2 seconds
	flags := supervisor.NewSupFlags(
		supervisor.SetStrategy(supervisor.OneForOne),
		supervisor.SetIntensity(1),
		supervisor.SetPeriod(2),
	)

	var initCount atomic.Int32
	tr.Expect(childInitMsg{}, gomock.Any()).AnyTimes().Do(func(arg erltest.ExpectArg) {
		initCount.Add(1)
	})
	tr.Expect(childTerminateMsg{}, gomock.Any()).AnyTimes()

	supPID := startTestSupervisor(t, flags, childSpecs)
	assert.Assert(t, erl.IsAlive(supPID))

	tr.WaitOnFunc(func() bool {
		return initCount.Load() >= 1
	})

	child1PID, _ := erl.WhereIs("child1-name")
	initCount.Store(0)

	// First restart
	erl.Exit(trPID, child1PID, exitreason.Kill)

	tr.WaitOnFunc(func() bool {
		return initCount.Load() >= 1
	})

	// Wait for period to expire
	time.Sleep(3 * time.Second)

	child1PIDAfter, _ := erl.WhereIs("child1-name")
	initCount.Store(0)

	// Second restart should succeed (counter reset)
	erl.Exit(trPID, child1PIDAfter, exitreason.Kill)

	tr.WaitOnFunc(func() bool {
		return initCount.Load() >= 1
	})

	// Supervisor should still be alive
	assert.Assert(t, erl.IsAlive(supPID), "supervisor should survive - intensity counter should have reset")
}

// =============================================================================
// Strategy with Mixed Restart Types
// =============================================================================

func TestOneForAll_MixedRestartTypes(t *testing.T) {
	trPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(5*time.Second))

	childSpecs := []supervisor.ChildSpec{
		makeChildSpec("perm1", "perm1-name", trPID, supervisor.Permanent),
		makeChildSpec("trans1", "trans1-name", trPID, supervisor.Transient),
		makeChildSpec("temp1", "temp1-name", trPID, supervisor.Temporary),
	}

	flags := supervisor.NewSupFlags(
		supervisor.SetStrategy(supervisor.OneForAll),
		supervisor.SetIntensity(5),
		supervisor.SetPeriod(10),
	)

	var initCount atomic.Int32
	tr.Expect(childInitMsg{}, gomock.Any()).AnyTimes().Do(func(arg erltest.ExpectArg) {
		initCount.Add(1)
	})
	tr.Expect(childTerminateMsg{}, gomock.Any()).AnyTimes()

	supPID := startTestSupervisor(t, flags, childSpecs)
	assert.Assert(t, erl.IsAlive(supPID))

	tr.WaitOnFunc(func() bool {
		return initCount.Load() >= 3
	})

	permPID, _ := erl.WhereIs("perm1-name")
	initCount.Store(0)

	// When a child crashes in OneForAll:
	// - All children are stopped (Temporary removed from list)
	// - All remaining children (Permanent, Transient) are restarted
	// - Temporary is NOT restarted (removed from supervisor)
	//
	// Note: In OneForAll, the restart type only determines IF a child's exit
	// triggers a restart. Once triggered, ALL non-Temporary children restart.
	erl.Exit(trPID, permPID, exitreason.Kill)

	// Wait for perm1 and trans1 to restart (2 inits expected)
	tr.WaitOnFunc(func() bool {
		return initCount.Load() >= 2
	})

	// Give a bit more time for registration to complete
	time.Sleep(100 * time.Millisecond)

	// Permanent should have restarted
	_, ok := erl.WhereIs("perm1-name")
	assert.Assert(t, ok, "permanent child should have restarted")

	// Transient should have restarted (OneForAll restarts all non-Temporary children)
	_, ok = erl.WhereIs("trans1-name")
	assert.Assert(t, ok, "transient child should have restarted in OneForAll")

	// Temporary should NOT have restarted
	_, ok = erl.WhereIs("temp1-name")
	assert.Assert(t, !ok, "temporary child should NOT have restarted")

	// Supervisor should still be alive
	assert.Assert(t, erl.IsAlive(supPID))
}

func TestRestForOne_MixedRestartTypes(t *testing.T) {
	trPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(5*time.Second))

	childSpecs := []supervisor.ChildSpec{
		makeChildSpec("perm1", "perm1-name", trPID, supervisor.Permanent),
		makeChildSpec("trans1", "trans1-name", trPID, supervisor.Transient),
		makeChildSpec("temp1", "temp1-name", trPID, supervisor.Temporary),
		makeChildSpec("perm2", "perm2-name", trPID, supervisor.Permanent),
	}

	flags := supervisor.NewSupFlags(
		supervisor.SetStrategy(supervisor.RestForOne),
		supervisor.SetIntensity(5),
		supervisor.SetPeriod(10),
	)

	var initCount atomic.Int32
	tr.Expect(childInitMsg{}, gomock.Any()).AnyTimes().Do(func(arg erltest.ExpectArg) {
		initCount.Add(1)
	})
	tr.Expect(childTerminateMsg{}, gomock.Any()).AnyTimes()

	supPID := startTestSupervisor(t, flags, childSpecs)
	assert.Assert(t, erl.IsAlive(supPID))

	tr.WaitOnFunc(func() bool {
		return initCount.Load() >= 4
	})

	perm1PID, _ := erl.WhereIs("perm1-name")
	transPID, _ := erl.WhereIs("trans1-name")
	initCount.Store(0)

	// When transient child crashes (killed):
	// - trans1, temp1, perm2 are terminated by supervisor
	// - trans1 restarts (it was killed abnormally)
	// - perm2 restarts (permanent always restarts)
	// - temp1 does NOT restart (temporary never restarts)
	// - perm1 is NOT affected (earlier in child order)
	//
	// Note: In RestForOne, the crashed child and all children started after it
	// are restarted. Siblings terminated by supervisor get normal/shutdown exit.
	// But perm2 still restarts because permanent children always restart.
	erl.Exit(trPID, transPID, exitreason.Kill)

	// Wait for trans1 and perm2 to restart (2 inits expected)
	tr.WaitOnFunc(func() bool {
		return initCount.Load() >= 2
	})

	// Give a bit more time for any additional restarts
	time.Sleep(100 * time.Millisecond)

	// perm1 should have SAME PID (not affected by RestForOne)
	perm1PIDAfter, _ := erl.WhereIs("perm1-name")
	assert.Assert(t, perm1PID.Equals(perm1PIDAfter), "perm1 should NOT have been restarted")

	// trans1 should have restarted (it was the crashed child)
	_, ok := erl.WhereIs("trans1-name")
	assert.Assert(t, ok, "transient child should have restarted (it was killed)")

	// temp1 should NOT have restarted
	_, ok = erl.WhereIs("temp1-name")
	assert.Assert(t, !ok, "temporary child should NOT have restarted")

	// perm2 should have restarted (permanent always restarts)
	_, ok = erl.WhereIs("perm2-name")
	assert.Assert(t, ok, "perm2 should have restarted")

	// Supervisor should still be alive
	assert.Assert(t, erl.IsAlive(supPID))
}
