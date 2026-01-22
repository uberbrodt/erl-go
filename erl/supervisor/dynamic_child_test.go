package supervisor

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
	"github.com/uberbrodt/erl-go/erl/genserver"
)

// =============================================================================
// WhichChildren Tests
// =============================================================================

func TestWhichChildren_ReturnsAllChildren(t *testing.T) {
	sup := TestSup{
		supFlags: NewSupFlags(),
		childSpecs: []ChildSpec{
			NewChildSpec("child1", testSrvStartFun[TestSrvState](supChild1, nil)),
			NewChildSpec("child2", testSrvStartFun[TestSrvState](supChild2, nil)),
		},
	}

	supPID, err := testStartSupervisor(t, sup, nil)
	assert.NilError(t, err)

	children, err := WhichChildren(supPID)
	assert.NilError(t, err)
	assert.Equal(t, len(children), 2)

	// Verify child1
	assert.Equal(t, children[0].ID, "child1")
	assert.Equal(t, children[0].Status, ChildRunning)
	assert.Assert(t, erl.IsAlive(children[0].PID))
	assert.Equal(t, children[0].Type, WorkerChild)
	assert.Equal(t, children[0].Restart, Permanent)

	// Verify child2
	assert.Equal(t, children[1].ID, "child2")
	assert.Equal(t, children[1].Status, ChildRunning)
	assert.Assert(t, erl.IsAlive(children[1].PID))
}

func TestWhichChildren_ShowsIgnoredStatus(t *testing.T) {
	sup := TestSup{
		supFlags: NewSupFlags(),
		childSpecs: []ChildSpec{
			NewChildSpec("child1", testSrvStartFun[TestSrvState](supChild1, nil)),
			NewChildSpec("ignored_child",
				testSrvStartFun[TestSrvState](supChild2, nil,
					genserver.SetInitProbe(
						func(self erl.PID, args any) (TestSrvState, any, error) {
							return TestSrvState{}, nil, exitreason.Ignore
						},
					))),
		},
	}

	supPID, err := testStartSupervisor(t, sup, nil)
	assert.NilError(t, err)

	children, err := WhichChildren(supPID)
	assert.NilError(t, err)
	assert.Equal(t, len(children), 2)

	// Verify ignored child
	assert.Equal(t, children[1].ID, "ignored_child")
	assert.Equal(t, children[1].Status, ChildUndefined)
	assert.Equal(t, children[1].PID, erl.PID{}) // Zero PID
}

func TestWhichChildren_ShowsTerminatedStatus(t *testing.T) {
	sup := TestSup{
		supFlags: NewSupFlags(),
		childSpecs: []ChildSpec{
			NewChildSpec("child1", testSrvStartFun[TestSrvState](supChild1, nil)),
		},
	}

	supPID, err := testStartSupervisor(t, sup, nil)
	assert.NilError(t, err)

	// Terminate the child
	err = TerminateChild(supPID, "child1")
	assert.NilError(t, err)

	children, err := WhichChildren(supPID)
	assert.NilError(t, err)
	assert.Equal(t, len(children), 1)

	// Verify terminated child
	assert.Equal(t, children[0].ID, "child1")
	assert.Equal(t, children[0].Status, ChildTerminated)
	assert.Equal(t, children[0].PID, erl.PID{}) // Zero PID
}

// =============================================================================
// CountChildren Tests
// =============================================================================

func TestCountChildren_ReturnsCorrectCounts(t *testing.T) {
	sup := TestSup{
		supFlags: NewSupFlags(),
		childSpecs: []ChildSpec{
			NewChildSpec("worker1", testSrvStartFun[TestSrvState](supChild1, nil)),
			NewChildSpec("worker2", testSrvStartFun[TestSrvState](supChild2, nil)),
			NewChildSpec("sub_sup", testSrvStartFun[TestSrvState](supChild3, nil),
				SetChildType(SupervisorChild)),
		},
	}

	supPID, err := testStartSupervisor(t, sup, nil)
	assert.NilError(t, err)

	count, err := CountChildren(supPID)
	assert.NilError(t, err)
	assert.Equal(t, count.Specs, 3)
	assert.Equal(t, count.Active, 3)
	assert.Equal(t, count.Workers, 2)
	assert.Equal(t, count.Supervisors, 1)
}

func TestCountChildren_CountsTerminatedCorrectly(t *testing.T) {
	sup := TestSup{
		supFlags: NewSupFlags(),
		childSpecs: []ChildSpec{
			NewChildSpec("child1", testSrvStartFun[TestSrvState](supChild1, nil)),
			NewChildSpec("child2", testSrvStartFun[TestSrvState](supChild2, nil)),
		},
	}

	supPID, err := testStartSupervisor(t, sup, nil)
	assert.NilError(t, err)

	// Terminate one child
	err = TerminateChild(supPID, "child1")
	assert.NilError(t, err)

	count, err := CountChildren(supPID)
	assert.NilError(t, err)
	assert.Equal(t, count.Specs, 2)   // Still have 2 specs
	assert.Equal(t, count.Active, 1)  // Only 1 active
	assert.Equal(t, count.Workers, 2) // Both are workers
}

// =============================================================================
// StartChild Tests
// =============================================================================

func TestStartChild_AddsAndStartsNewChild(t *testing.T) {
	sup := TestSup{
		supFlags:   NewSupFlags(),
		childSpecs: []ChildSpec{},
	}

	supPID, err := testStartSupervisor(t, sup, nil)
	assert.NilError(t, err)

	// Verify no children initially
	count, _ := CountChildren(supPID)
	assert.Equal(t, count.Specs, 0)

	// Start a new child dynamically
	spec := NewChildSpec("dynamic_child", testSrvStartFun[TestSrvState](supChild1, nil))
	childPID, err := StartChild(supPID, spec)

	assert.NilError(t, err)
	assert.Assert(t, erl.IsAlive(childPID))

	// Verify child is tracked
	count, _ = CountChildren(supPID)
	assert.Equal(t, count.Specs, 1)
	assert.Equal(t, count.Active, 1)

	// Verify in WhichChildren
	children, _ := WhichChildren(supPID)
	assert.Equal(t, len(children), 1)
	assert.Equal(t, children[0].ID, "dynamic_child")
	assert.Equal(t, children[0].Status, ChildRunning)
}

func TestStartChild_ReturnsAlreadyStartedForRunningChild(t *testing.T) {
	sup := TestSup{
		supFlags: NewSupFlags(),
		childSpecs: []ChildSpec{
			NewChildSpec("child1", testSrvStartFun[TestSrvState](supChild1, nil)),
		},
	}

	supPID, err := testStartSupervisor(t, sup, nil)
	assert.NilError(t, err)

	// Get original PID
	children, _ := WhichChildren(supPID)
	originalPID := children[0].PID

	// Try to start another child with same ID
	spec := NewChildSpec("child1", testSrvStartFun[TestSrvState](supChild2, nil))
	_, err = StartChild(supPID, spec)

	assert.Assert(t, errors.Is(err, ErrAlreadyStarted))

	// Verify we can get the existing PID from the error
	var asErr AlreadyStartedError
	assert.Assert(t, errors.As(err, &asErr))
	assert.Assert(t, asErr.PID.Equals(originalPID))
}

func TestStartChild_ReturnsAlreadyPresentForTerminatedChild(t *testing.T) {
	sup := TestSup{
		supFlags: NewSupFlags(),
		childSpecs: []ChildSpec{
			NewChildSpec("child1", testSrvStartFun[TestSrvState](supChild1, nil)),
		},
	}

	supPID, err := testStartSupervisor(t, sup, nil)
	assert.NilError(t, err)

	// Terminate the child first
	err = TerminateChild(supPID, "child1")
	assert.NilError(t, err)

	// Try to start another child with same ID
	spec := NewChildSpec("child1", testSrvStartFun[TestSrvState](supChild2, nil))
	_, err = StartChild(supPID, spec)

	assert.Assert(t, errors.Is(err, ErrAlreadyPresent))
}

func TestStartChild_DoesNotAffectRestartIntensity(t *testing.T) {
	// Supervisor with intensity=1, period=5 - very strict
	sup := TestSup{
		supFlags:   NewSupFlags(SetIntensity(1), SetPeriod(5)),
		childSpecs: []ChildSpec{},
	}

	supPID, err := testStartSupervisor(t, sup, nil)
	assert.NilError(t, err)

	// Start multiple children dynamically - should not trip intensity
	for i := 0; i < 5; i++ {
		name := erl.Name(fmt.Sprintf("dynamic-child-%d", i))
		spec := NewChildSpec(
			fmt.Sprintf("child%d", i),
			testSrvStartFun[TestSrvState](name, nil),
		)
		_, err := StartChild(supPID, spec)
		assert.NilError(t, err)
	}

	// Supervisor should still be alive
	assert.Assert(t, erl.IsAlive(supPID))

	count, _ := CountChildren(supPID)
	assert.Equal(t, count.Active, 5)
}

// =============================================================================
// TerminateChild Tests
// =============================================================================

func TestTerminateChild_StopsRunningChild(t *testing.T) {
	sup := TestSup{
		supFlags: NewSupFlags(),
		childSpecs: []ChildSpec{
			NewChildSpec("child1", testSrvStartFun[TestSrvState](supChild1, nil)),
		},
	}

	supPID, err := testStartSupervisor(t, sup, nil)
	assert.NilError(t, err)

	child1PID, _ := erl.WhereIs(supChild1)
	assert.Assert(t, erl.IsAlive(child1PID))

	err = TerminateChild(supPID, "child1")
	assert.NilError(t, err)

	// Child should be stopped
	// Give a moment for process to terminate
	time.Sleep(50 * time.Millisecond)
	assert.Assert(t, !erl.IsAlive(child1PID))

	// But spec should still exist
	children, _ := WhichChildren(supPID)
	assert.Equal(t, len(children), 1)
	assert.Equal(t, children[0].Status, ChildTerminated)
}

func TestTerminateChild_ReturnsNotFoundForUnknownChild(t *testing.T) {
	sup := TestSup{
		supFlags:   NewSupFlags(),
		childSpecs: []ChildSpec{},
	}

	supPID, err := testStartSupervisor(t, sup, nil)
	assert.NilError(t, err)

	err = TerminateChild(supPID, "nonexistent")
	assert.Assert(t, errors.Is(err, ErrNotFound))
}

func TestTerminateChild_IdempotentOnTerminatedChild(t *testing.T) {
	sup := TestSup{
		supFlags: NewSupFlags(),
		childSpecs: []ChildSpec{
			NewChildSpec("child1", testSrvStartFun[TestSrvState](supChild1, nil)),
		},
	}

	supPID, err := testStartSupervisor(t, sup, nil)
	assert.NilError(t, err)

	// Terminate twice - should be idempotent
	err = TerminateChild(supPID, "child1")
	assert.NilError(t, err)

	err = TerminateChild(supPID, "child1")
	assert.NilError(t, err) // No error on second call
}

func TestTerminateChild_PreventsAutoRestart(t *testing.T) {
	sup := TestSup{
		supFlags: NewSupFlags(SetIntensity(10), SetPeriod(60)), // Allow many restarts
		childSpecs: []ChildSpec{
			NewChildSpec("child1",
				testSrvStartFun[TestSrvState](supChild1, nil),
				SetRestart(Permanent)), // Would normally always restart
		},
	}

	supPID, err := testStartSupervisor(t, sup, nil)
	assert.NilError(t, err)

	// Terminate via API
	err = TerminateChild(supPID, "child1")
	assert.NilError(t, err)

	// Wait a bit to ensure no auto-restart
	time.Sleep(100 * time.Millisecond)

	// Child should remain terminated (not restarted)
	children, _ := WhichChildren(supPID)
	assert.Equal(t, children[0].Status, ChildTerminated)

	// Supervisor should still be running (no intensity violation)
	assert.Assert(t, erl.IsAlive(supPID))
}

// =============================================================================
// RestartChild Tests
// =============================================================================

func TestRestartChild_RestartsTerminatedChild(t *testing.T) {
	sup := TestSup{
		supFlags: NewSupFlags(),
		childSpecs: []ChildSpec{
			NewChildSpec("child1", testSrvStartFun[TestSrvState](supChild1, nil)),
		},
	}

	supPID, err := testStartSupervisor(t, sup, nil)
	assert.NilError(t, err)

	originalPID, _ := erl.WhereIs(supChild1)

	// Terminate then restart
	err = TerminateChild(supPID, "child1")
	assert.NilError(t, err)

	newPID, err := RestartChild(supPID, "child1")
	assert.NilError(t, err)
	assert.Assert(t, erl.IsAlive(newPID))
	assert.Assert(t, !originalPID.Equals(newPID)) // Should be new PID

	// Status should be running again
	children, _ := WhichChildren(supPID)
	assert.Equal(t, children[0].Status, ChildRunning)
	assert.Assert(t, children[0].PID.Equals(newPID))
}

func TestRestartChild_ReturnsRunningForRunningChild(t *testing.T) {
	sup := TestSup{
		supFlags: NewSupFlags(),
		childSpecs: []ChildSpec{
			NewChildSpec("child1", testSrvStartFun[TestSrvState](supChild1, nil)),
		},
	}

	supPID, err := testStartSupervisor(t, sup, nil)
	assert.NilError(t, err)

	_, err = RestartChild(supPID, "child1")
	assert.Assert(t, errors.Is(err, ErrRunning))
}

func TestRestartChild_ReturnsNotFoundForUnknownChild(t *testing.T) {
	sup := TestSup{
		supFlags:   NewSupFlags(),
		childSpecs: []ChildSpec{},
	}

	supPID, err := testStartSupervisor(t, sup, nil)
	assert.NilError(t, err)

	_, err = RestartChild(supPID, "nonexistent")
	assert.Assert(t, errors.Is(err, ErrNotFound))
}

func TestRestartChild_CanRestartIgnoredChild(t *testing.T) {
	// Start a child that ignores, then restart it with a working implementation
	var startCount int
	sup := TestSup{
		supFlags: NewSupFlags(),
		childSpecs: []ChildSpec{
			NewChildSpec("child1", func(sup erl.PID) (erl.PID, error) {
				startCount++
				if startCount == 1 {
					// First time: return Ignore
					return erl.PID{}, exitreason.Ignore
				}
				// Subsequent times: actually start
				return genserver.StartLink[TestSrvState](sup,
					genserver.NewTestServer[TestSrvState](), nil,
					genserver.SetName(supChild1))
			}),
		},
	}

	supPID, err := testStartSupervisor(t, sup, nil)
	assert.NilError(t, err)

	// Verify initially ignored
	children, _ := WhichChildren(supPID)
	assert.Equal(t, children[0].Status, ChildUndefined)

	// Restart - should now actually start
	newPID, err := RestartChild(supPID, "child1")
	assert.NilError(t, err)
	assert.Assert(t, erl.IsAlive(newPID))

	// Verify now running
	children, _ = WhichChildren(supPID)
	assert.Equal(t, children[0].Status, ChildRunning)
}

// =============================================================================
// DeleteChild Tests
// =============================================================================

func TestDeleteChild_RemovesTerminatedChildSpec(t *testing.T) {
	sup := TestSup{
		supFlags: NewSupFlags(),
		childSpecs: []ChildSpec{
			NewChildSpec("child1", testSrvStartFun[TestSrvState](supChild1, nil)),
			NewChildSpec("child2", testSrvStartFun[TestSrvState](supChild2, nil)),
		},
	}

	supPID, err := testStartSupervisor(t, sup, nil)
	assert.NilError(t, err)

	// Terminate then delete
	err = TerminateChild(supPID, "child1")
	assert.NilError(t, err)

	err = DeleteChild(supPID, "child1")
	assert.NilError(t, err)

	// Spec should be gone
	children, _ := WhichChildren(supPID)
	assert.Equal(t, len(children), 1)
	assert.Equal(t, children[0].ID, "child2")

	count, _ := CountChildren(supPID)
	assert.Equal(t, count.Specs, 1)
}

func TestDeleteChild_ReturnsRunningForRunningChild(t *testing.T) {
	sup := TestSup{
		supFlags: NewSupFlags(),
		childSpecs: []ChildSpec{
			NewChildSpec("child1", testSrvStartFun[TestSrvState](supChild1, nil)),
		},
	}

	supPID, err := testStartSupervisor(t, sup, nil)
	assert.NilError(t, err)

	err = DeleteChild(supPID, "child1")
	assert.Assert(t, errors.Is(err, ErrRunning))

	// Child should still be there
	children, _ := WhichChildren(supPID)
	assert.Equal(t, len(children), 1)
}

func TestDeleteChild_ReturnsNotFoundForUnknownChild(t *testing.T) {
	sup := TestSup{
		supFlags:   NewSupFlags(),
		childSpecs: []ChildSpec{},
	}

	supPID, err := testStartSupervisor(t, sup, nil)
	assert.NilError(t, err)

	err = DeleteChild(supPID, "nonexistent")
	assert.Assert(t, errors.Is(err, ErrNotFound))
}

func TestDeleteChild_CanDeleteIgnoredChild(t *testing.T) {
	sup := TestSup{
		supFlags: NewSupFlags(),
		childSpecs: []ChildSpec{
			NewChildSpec("ignored_child",
				testSrvStartFun[TestSrvState](supChild1, nil,
					genserver.SetInitProbe(
						func(self erl.PID, args any) (TestSrvState, any, error) {
							return TestSrvState{}, nil, exitreason.Ignore
						},
					))),
		},
	}

	supPID, err := testStartSupervisor(t, sup, nil)
	assert.NilError(t, err)

	// Delete ignored child (not running, so should work)
	err = DeleteChild(supPID, "ignored_child")
	assert.NilError(t, err)

	// Should be gone
	children, _ := WhichChildren(supPID)
	assert.Equal(t, len(children), 0)
}

// =============================================================================
// Integration Tests
// =============================================================================

func TestDynamicChildLifecycle(t *testing.T) {
	// Test the complete lifecycle: start -> terminate -> restart -> terminate -> delete
	sup := TestSup{
		supFlags:   NewSupFlags(),
		childSpecs: []ChildSpec{},
	}

	supPID, err := testStartSupervisor(t, sup, nil)
	assert.NilError(t, err)

	// 1. Start a child dynamically
	spec := NewChildSpec("lifecycle_child", testSrvStartFun[TestSrvState](supChild1, nil))
	pid1, err := StartChild(supPID, spec)
	assert.NilError(t, err)
	assert.Assert(t, erl.IsAlive(pid1))

	// 2. Terminate it
	err = TerminateChild(supPID, "lifecycle_child")
	assert.NilError(t, err)
	time.Sleep(50 * time.Millisecond)
	assert.Assert(t, !erl.IsAlive(pid1))

	// 3. Restart it
	pid2, err := RestartChild(supPID, "lifecycle_child")
	assert.NilError(t, err)
	assert.Assert(t, erl.IsAlive(pid2))
	assert.Assert(t, !pid1.Equals(pid2)) // Different PID

	// 4. Terminate again
	err = TerminateChild(supPID, "lifecycle_child")
	assert.NilError(t, err)

	// 5. Delete the spec
	err = DeleteChild(supPID, "lifecycle_child")
	assert.NilError(t, err)

	// Verify fully removed
	count, _ := CountChildren(supPID)
	assert.Equal(t, count.Specs, 0)
}
