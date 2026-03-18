package dynsup_test

import (
	"errors"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/uberbrodt/erl-go/chronos"
	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/dynsup"
	"github.com/uberbrodt/erl-go/erl/exitreason"
	"github.com/uberbrodt/erl-go/erl/genserver"
	"github.com/uberbrodt/erl-go/erl/supervisor"
)

// =============================================================================
// Test helpers
// =============================================================================

const testSupName erl.Name = "test-dynsup"

type testDynSup struct {
	flags  dynsup.SupFlags
	ignore bool
}

func (s testDynSup) Init(self erl.PID, args any) dynsup.InitResult {
	return dynsup.InitResult{SupFlags: s.flags, Ignore: s.ignore}
}

type workerState struct {
	id string
}

// workerStartFun creates a start function for a simple test GenServer worker.
func workerStartFun(name erl.Name, trPID erl.PID) supervisor.StartFunSpec {
	ts := genserver.NewTestServer[workerState](
		genserver.SetTestReceiver[workerState](trPID),
		genserver.SetInitialState[workerState](workerState{id: string(name)}),
	)
	return func(sup erl.PID) (erl.PID, error) {
		return genserver.StartLink[workerState](sup, ts, nil, genserver.SetName(name))
	}
}

// startTestDynSup starts a named dynamic supervisor and registers cleanup.
func startTestDynSup(t *testing.T, flags dynsup.SupFlags) erl.PID {
	t.Helper()
	pid, err := dynsup.StartDefaultLink(erl.RootPID(), flags, dynsup.SetName(testSupName))
	assert.NilError(t, err)
	assert.Assert(t, erl.IsAlive(pid))
	t.Cleanup(func() {
		genserver.Stop(erl.RootPID(), pid, genserver.StopReason(exitreason.SupervisorShutdown))
	})
	return pid
}

// waitForExit waits until the given PID appears in an ExitMsg on the TestReceiver.
func waitForExit(t *testing.T, tr *erl.TestReceiver, pid erl.PID) {
	t.Helper()
	tr.Loop(func(msg any) bool {
		if exit, ok := msg.(erl.ExitMsg); ok {
			return exit.Proc.Equals(pid)
		}
		return false
	})
}

// waitForInit waits until a TestNotifInit with the given id arrives.
func waitForInit(t *testing.T, tr *erl.TestReceiver, id string) erl.PID {
	t.Helper()
	var initPID erl.PID
	tr.Loop(func(msg any) bool {
		if notif, ok := msg.(genserver.TestNotifInit[workerState]); ok {
			if notif.State.id == id {
				initPID = notif.Self
				return true
			}
		}
		return false
	})
	return initPID
}

// =============================================================================
// 5.1 Test StartLink
// =============================================================================

func TestStartLink_BasicStartup(t *testing.T) {
	trPID, _ := erl.NewTestReceiver(t)

	sup := testDynSup{flags: dynsup.NewSupFlags()}
	pid, err := dynsup.StartLink(trPID, sup, nil, dynsup.SetName(testSupName))
	assert.NilError(t, err)
	assert.Assert(t, erl.IsAlive(pid))

	foundPID, ok := erl.WhereIs(testSupName)
	assert.Assert(t, ok)
	assert.Assert(t, pid.Equals(foundPID))

	t.Cleanup(func() {
		genserver.Stop(erl.RootPID(), pid, genserver.StopReason(exitreason.SupervisorShutdown))
	})
}

func TestStartLink_NamedRegistration(t *testing.T) {
	trPID, _ := erl.NewTestReceiver(t)

	name := erl.Name("named-dynsup")
	sup := testDynSup{flags: dynsup.NewSupFlags()}
	pid, err := dynsup.StartLink(trPID, sup, nil, dynsup.SetName(name))
	assert.NilError(t, err)

	foundPID, ok := erl.WhereIs(name)
	assert.Assert(t, ok)
	assert.Assert(t, pid.Equals(foundPID))

	t.Cleanup(func() {
		genserver.Stop(erl.RootPID(), pid, genserver.StopReason(exitreason.SupervisorShutdown))
	})
}

func TestStartLink_IgnoreFromInit(t *testing.T) {
	trPID, _ := erl.NewTestReceiver(t)

	sup := testDynSup{flags: dynsup.NewSupFlags(), ignore: true}
	_, err := dynsup.StartLink(trPID, sup, nil)
	assert.Assert(t, errors.Is(err, exitreason.Ignore))
}

// =============================================================================
// 5.2 Test StartDefaultLink
// =============================================================================

func TestStartDefaultLink_BasicStartup(t *testing.T) {
	pid := startTestDynSup(t, dynsup.NewSupFlags())
	assert.Assert(t, erl.IsAlive(pid))

	foundPID, ok := erl.WhereIs(testSupName)
	assert.Assert(t, ok)
	assert.Assert(t, pid.Equals(foundPID))
}

// =============================================================================
// 5.3 Test StartChild
// =============================================================================

func TestStartChild_SuccessfulStart(t *testing.T) {
	trPID, _ := erl.NewTestReceiver(t)
	supPID := startTestDynSup(t, dynsup.NewSupFlags())

	spec := dynsup.NewChildSpec(workerStartFun("sc-child1", trPID))
	childPID, err := dynsup.StartChild(supPID, spec)
	assert.NilError(t, err)
	assert.Assert(t, erl.IsAlive(childPID))

	foundPID, ok := erl.WhereIs("sc-child1")
	assert.Assert(t, ok)
	assert.Assert(t, childPID.Equals(foundPID))
}

func TestStartChild_MaxChildrenEnforcement(t *testing.T) {
	trPID, _ := erl.NewTestReceiver(t)
	supPID := startTestDynSup(t, dynsup.NewSupFlags(dynsup.SetMaxChildren(1)))

	spec1 := dynsup.NewChildSpec(workerStartFun("mc-child1", trPID))
	_, err := dynsup.StartChild(supPID, spec1)
	assert.NilError(t, err)

	spec2 := dynsup.NewChildSpec(workerStartFun("mc-child2", trPID))
	_, err = dynsup.StartChild(supPID, spec2)
	assert.Assert(t, errors.Is(err, dynsup.ErrMaxChildren))
}

func TestStartChild_StartFailure(t *testing.T) {
	supPID := startTestDynSup(t, dynsup.NewSupFlags())

	spec := dynsup.NewChildSpec(func(sup erl.PID) (erl.PID, error) {
		return erl.PID{}, errors.New("start failed")
	})
	_, err := dynsup.StartChild(supPID, spec)
	assert.ErrorContains(t, err, "start failed")
	assert.Assert(t, erl.IsAlive(supPID))
}

func TestStartChild_PanicRecovery(t *testing.T) {
	supPID := startTestDynSup(t, dynsup.NewSupFlags())

	spec := dynsup.NewChildSpec(func(sup erl.PID) (erl.PID, error) {
		panic("boom")
	})
	_, err := dynsup.StartChild(supPID, spec)
	assert.ErrorContains(t, err, "boom")
	assert.Assert(t, erl.IsAlive(supPID))
}

func TestStartChild_IgnoreHandling(t *testing.T) {
	supPID := startTestDynSup(t, dynsup.NewSupFlags())

	spec := dynsup.NewChildSpec(func(sup erl.PID) (erl.PID, error) {
		return erl.PID{}, exitreason.Ignore
	})
	pid, err := dynsup.StartChild(supPID, spec)
	assert.NilError(t, err)
	assert.Assert(t, !erl.IsAlive(pid))

	// Supervisor should still be alive and have 0 children
	count, err := dynsup.CountChildren(supPID)
	assert.NilError(t, err)
	assert.Equal(t, count.Specs, 0)
}

// =============================================================================
// 5.4 Test TerminateChild
// =============================================================================

func TestTerminateChild_SuccessfulTermination(t *testing.T) {
	trPID, _ := erl.NewTestReceiver(t)
	supPID := startTestDynSup(t, dynsup.NewSupFlags())

	spec := dynsup.NewChildSpec(workerStartFun("tc-child1", trPID))
	childPID, err := dynsup.StartChild(supPID, spec)
	assert.NilError(t, err)
	assert.Assert(t, erl.IsAlive(childPID))

	err = dynsup.TerminateChild(supPID, childPID)
	assert.NilError(t, err)

	// Child should be dead and removed
	time.Sleep(chronos.Dur("100ms"))
	assert.Assert(t, !erl.IsAlive(childPID))

	count, err := dynsup.CountChildren(supPID)
	assert.NilError(t, err)
	assert.Equal(t, count.Specs, 0)
}

func TestTerminateChild_PIDNotFound(t *testing.T) {
	supPID := startTestDynSup(t, dynsup.NewSupFlags())

	// Try to terminate a PID that isn't a child
	fakePID := erl.Spawn(&noopProcess{})
	err := dynsup.TerminateChild(supPID, fakePID)
	assert.Assert(t, errors.Is(err, dynsup.ErrNotFound))
}

// =============================================================================
// 5.5 Test automatic restart
// =============================================================================

func TestRestart_PermanentAlwaysRestarts(t *testing.T) {
	trPID, tr := erl.NewTestReceiver(t)
	supPID := startTestDynSup(t, dynsup.NewSupFlags(dynsup.SetIntensity(5)))

	spec := dynsup.NewChildSpec(
		workerStartFun("perm-child", trPID),
		supervisor.SetRestart(supervisor.Permanent),
	)
	childPID, err := dynsup.StartChild(supPID, spec)
	assert.NilError(t, err)

	// Wait for initial init notification
	waitForInit(t, tr, "perm-child")

	// Kill the child — it should restart since it's Permanent
	erl.Exit(erl.RootPID(), childPID, exitreason.Kill)
	waitForExit(t, tr, childPID)

	// Wait for restart init
	newPID := waitForInit(t, tr, "perm-child")
	assert.Assert(t, !childPID.Equals(newPID))
	assert.Assert(t, erl.IsAlive(newPID))
	assert.Assert(t, erl.IsAlive(supPID))
}

func TestRestart_TransientRestartsOnAbnormalExit(t *testing.T) {
	trPID, tr := erl.NewTestReceiver(t)
	supPID := startTestDynSup(t, dynsup.NewSupFlags(dynsup.SetIntensity(5)))

	spec := dynsup.NewChildSpec(
		workerStartFun("trans-child", trPID),
		supervisor.SetRestart(supervisor.Transient),
	)
	childPID, err := dynsup.StartChild(supPID, spec)
	assert.NilError(t, err)

	waitForInit(t, tr, "trans-child")

	// Kill (abnormal) — should restart
	erl.Exit(erl.RootPID(), childPID, exitreason.Kill)
	waitForExit(t, tr, childPID)

	newPID := waitForInit(t, tr, "trans-child")
	assert.Assert(t, !childPID.Equals(newPID))
	assert.Assert(t, erl.IsAlive(supPID))
}

func TestRestart_TransientDoesNotRestartOnNormalExit(t *testing.T) {
	trPID, tr := erl.NewTestReceiver(t)
	supPID := startTestDynSup(t, dynsup.NewSupFlags(dynsup.SetIntensity(5)))

	spec := dynsup.NewChildSpec(
		workerStartFun("trans-normal", trPID),
		supervisor.SetRestart(supervisor.Transient),
	)
	childPID, err := dynsup.StartChild(supPID, spec)
	assert.NilError(t, err)

	waitForInit(t, tr, "trans-normal")

	// Send normal exit via genserver call
	msg := genserver.NewTestMsg[workerState](genserver.SetCallProbe[workerState](
		func(self erl.PID, arg any, from genserver.From, state workerState) (genserver.CallResult[workerState], error) {
			return genserver.CallResult[workerState]{Msg: "bye", State: state}, exitreason.Normal
		},
	))
	result, err := genserver.Call(trPID, childPID, msg, chronos.Dur("5s"))
	assert.NilError(t, err)
	assert.DeepEqual(t, result, "bye")

	waitForExit(t, tr, childPID)

	// Supervisor should be alive, child count should be 0 (not restarted)
	assert.Assert(t, erl.IsAlive(supPID))
	count, err := dynsup.CountChildren(supPID)
	assert.NilError(t, err)
	assert.Equal(t, count.Specs, 0)
}

func TestRestart_TemporaryNeverRestarts(t *testing.T) {
	trPID, tr := erl.NewTestReceiver(t)
	supPID := startTestDynSup(t, dynsup.NewSupFlags(dynsup.SetIntensity(5)))

	spec := dynsup.NewChildSpec(
		workerStartFun("temp-child", trPID),
		supervisor.SetRestart(supervisor.Temporary),
	)
	childPID, err := dynsup.StartChild(supPID, spec)
	assert.NilError(t, err)

	waitForInit(t, tr, "temp-child")

	// Kill — should NOT restart
	erl.Exit(erl.RootPID(), childPID, exitreason.Kill)
	waitForExit(t, tr, childPID)

	// Give a moment for HandleInfo to process
	time.Sleep(chronos.Dur("200ms"))

	assert.Assert(t, erl.IsAlive(supPID))
	count, err := dynsup.CountChildren(supPID)
	assert.NilError(t, err)
	assert.Equal(t, count.Specs, 0)
}

// =============================================================================
// 5.6 Test restart failure
// =============================================================================

func TestRestartFailure_SupervisorTerminates(t *testing.T) {
	trPID, tr := erl.NewTestReceiver(t)

	flags := dynsup.NewSupFlags(dynsup.SetIntensity(5))
	supPID, err := dynsup.StartDefaultLink(trPID, flags, dynsup.SetName("rf-dynsup"))
	assert.NilError(t, err)

	var startCount int
	spec := dynsup.NewChildSpec(func(sup erl.PID) (erl.PID, error) {
		startCount++
		if startCount > 1 {
			// Restart attempt fails
			return erl.PID{}, errors.New("restart failed")
		}
		return genserver.StartLink[workerState](sup,
			genserver.NewTestServer[workerState](
				genserver.SetTestReceiver[workerState](trPID),
				genserver.SetInitialState[workerState](workerState{id: "rf-child"}),
			), nil)
	}, supervisor.SetRestart(supervisor.Permanent))

	childPID, err := dynsup.StartChild(supPID, spec)
	assert.NilError(t, err)

	waitForInit(t, tr, "rf-child")

	// Kill the child — restart will fail, supervisor should terminate
	erl.Exit(erl.RootPID(), childPID, exitreason.Kill)

	// Wait for supervisor to die
	waitForExit(t, tr, supPID)
	assert.Assert(t, !erl.IsAlive(supPID))
}

// =============================================================================
// 5.7 Test restart intensity
// =============================================================================

func TestRestartIntensity_SupervisorTerminatesWhenExceeded(t *testing.T) {
	trPID, tr := erl.NewTestReceiver(t)

	// Intensity 1, period 5s — second restart within window should kill supervisor
	flags := dynsup.NewSupFlags(dynsup.SetIntensity(1), dynsup.SetPeriod(5))
	supPID, err := dynsup.StartDefaultLink(trPID, flags, dynsup.SetName("ri-dynsup"))
	assert.NilError(t, err)

	spec1 := dynsup.NewChildSpec(
		workerStartFun("ri-child1", trPID),
		supervisor.SetRestart(supervisor.Permanent),
	)
	spec2 := dynsup.NewChildSpec(
		workerStartFun("ri-child2", trPID),
		supervisor.SetRestart(supervisor.Permanent),
	)

	child1PID, err := dynsup.StartChild(supPID, spec1)
	assert.NilError(t, err)
	waitForInit(t, tr, "ri-child1")

	child2PID, err := dynsup.StartChild(supPID, spec2)
	assert.NilError(t, err)
	waitForInit(t, tr, "ri-child2")

	// Kill both children rapidly
	erl.Exit(trPID, child1PID, exitreason.Kill)
	erl.Exit(trPID, child2PID, exitreason.Kill)

	// Supervisor should die from intensity exceeded
	waitForExit(t, tr, supPID)
	assert.Assert(t, !erl.IsAlive(supPID))
}

// =============================================================================
// 5.8 Test unmanaged ExitMsg
// =============================================================================

func TestUnmanagedExitMsg_SilentlyIgnored(t *testing.T) {
	trPID, _ := erl.NewTestReceiver(t)
	supPID := startTestDynSup(t, dynsup.NewSupFlags())

	// Start a child so we know the supervisor is working
	spec := dynsup.NewChildSpec(workerStartFun("ue-child1", trPID))
	childPID, err := dynsup.StartChild(supPID, spec)
	assert.NilError(t, err)

	// Spawn an unmanaged process and link it to the supervisor, then kill it
	unmanaged := erl.Spawn(&noopProcess{})
	erl.Link(supPID, unmanaged)
	erl.Exit(erl.RootPID(), unmanaged, exitreason.Kill)

	// Give supervisor time to process the ExitMsg
	time.Sleep(chronos.Dur("200ms"))

	// Supervisor should still be alive with its child
	assert.Assert(t, erl.IsAlive(supPID))
	assert.Assert(t, erl.IsAlive(childPID))

	count, err := dynsup.CountChildren(supPID)
	assert.NilError(t, err)
	assert.Equal(t, count.Specs, 1)
}

// =============================================================================
// 5.9 Test WhichChildren and CountChildren
// =============================================================================

func TestWhichChildren_CorrectInfoForMixedChildren(t *testing.T) {
	trPID, _ := erl.NewTestReceiver(t)
	supPID := startTestDynSup(t, dynsup.NewSupFlags())

	// Start a worker child
	spec1 := dynsup.NewChildSpec(workerStartFun("wc-worker", trPID))
	_, err := dynsup.StartChild(supPID, spec1)
	assert.NilError(t, err)

	// Start a "supervisor" child type
	spec2 := dynsup.NewChildSpec(
		workerStartFun("wc-sup", trPID),
		supervisor.SetChildType(supervisor.SupervisorChild),
	)
	_, err = dynsup.StartChild(supPID, spec2)
	assert.NilError(t, err)

	children, err := dynsup.WhichChildren(supPID)
	assert.NilError(t, err)
	assert.Equal(t, len(children), 2)

	var workers, sups int
	for _, c := range children {
		assert.Equal(t, c.Status, supervisor.ChildRunning)
		assert.Assert(t, erl.IsAlive(c.PID))
		if c.Type == supervisor.WorkerChild {
			workers++
		} else {
			sups++
		}
	}
	assert.Equal(t, workers, 1)
	assert.Equal(t, sups, 1)
}

func TestCountChildren_CorrectCounts(t *testing.T) {
	trPID, _ := erl.NewTestReceiver(t)
	supPID := startTestDynSup(t, dynsup.NewSupFlags())

	// Start 2 workers and 1 supervisor-type child
	for _, name := range []erl.Name{"cc-w1", "cc-w2"} {
		spec := dynsup.NewChildSpec(workerStartFun(name, trPID))
		_, err := dynsup.StartChild(supPID, spec)
		assert.NilError(t, err)
	}
	spec := dynsup.NewChildSpec(
		workerStartFun("cc-sup", trPID),
		supervisor.SetChildType(supervisor.SupervisorChild),
	)
	_, err := dynsup.StartChild(supPID, spec)
	assert.NilError(t, err)

	count, err := dynsup.CountChildren(supPID)
	assert.NilError(t, err)
	assert.Equal(t, count.Specs, 3)
	assert.Equal(t, count.Active, 3)
	assert.Equal(t, count.Workers, 2)
	assert.Equal(t, count.Supervisors, 1)
}

// =============================================================================
// 5.10 Test concurrent shutdown
// =============================================================================

func TestConcurrentShutdown_AllChildrenTerminated(t *testing.T) {
	trPID, tr := erl.NewTestReceiver(t)

	flags := dynsup.NewSupFlags()
	supPID, err := dynsup.StartDefaultLink(trPID, flags, dynsup.SetName("cs-dynsup"))
	assert.NilError(t, err)

	// Start several children
	childPIDs := make([]erl.PID, 5)
	for i := 0; i < 5; i++ {
		name := erl.Name("cs-child-" + string(rune('a'+i)))
		spec := dynsup.NewChildSpec(workerStartFun(name, trPID))
		pid, err := dynsup.StartChild(supPID, spec)
		assert.NilError(t, err)
		childPIDs[i] = pid
	}

	// Verify all alive
	for _, pid := range childPIDs {
		assert.Assert(t, erl.IsAlive(pid))
	}

	// Stop the supervisor
	genserver.Stop(erl.RootPID(), supPID, genserver.StopReason(exitreason.SupervisorShutdown))

	// Wait for supervisor exit
	waitForExit(t, tr, supPID)

	// All children should be dead
	for _, pid := range childPIDs {
		assert.Assert(t, !erl.IsAlive(pid))
	}
}

// =============================================================================
// 5.11 Test NewChildSpec
// =============================================================================

func TestNewChildSpec_CreatesSpecWithEmptyIDAndDefaults(t *testing.T) {
	startFn := func(sup erl.PID) (erl.PID, error) {
		return erl.PID{}, nil
	}
	spec := dynsup.NewChildSpec(startFn)

	assert.Equal(t, spec.ID, "")
	assert.Equal(t, spec.Restart, supervisor.Permanent)
	assert.Equal(t, spec.Shutdown, supervisor.ShutdownOpt{Timeout: 5_000})
	assert.Equal(t, spec.Type, supervisor.WorkerChild)
}

func TestNewChildSpec_AppliesOptions(t *testing.T) {
	startFn := func(sup erl.PID) (erl.PID, error) {
		return erl.PID{}, nil
	}
	spec := dynsup.NewChildSpec(startFn,
		supervisor.SetRestart(supervisor.Transient),
		supervisor.SetShutdown(supervisor.ShutdownOpt{Infinity: true}),
		supervisor.SetChildType(supervisor.SupervisorChild),
	)

	assert.Equal(t, spec.ID, "")
	assert.Equal(t, spec.Restart, supervisor.Transient)
	assert.Equal(t, spec.Shutdown, supervisor.ShutdownOpt{Infinity: true})
	assert.Equal(t, spec.Type, supervisor.SupervisorChild)
}

// =============================================================================
// Helpers
// =============================================================================

// noopProcess is a minimal process that does nothing (exits immediately on inbox close).
type noopProcess struct{}

func (n *noopProcess) Receive(self erl.PID, inbox <-chan any) error {
	for range inbox {
	}
	return exitreason.Normal
}
