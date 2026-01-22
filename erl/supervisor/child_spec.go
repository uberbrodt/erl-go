package supervisor

import "github.com/uberbrodt/erl-go/erl"

// ChildSpecOpt is a functional option for configuring a [ChildSpec].
// Use with [NewChildSpec] to customize child behavior.
type ChildSpecOpt func(cs ChildSpec) ChildSpec

// SetRestart sets when the child should be restarted after termination.
// Default is [Permanent] (always restart).
//
// Available options:
//   - [Permanent]: Always restart regardless of exit reason
//   - [Transient]: Restart only on abnormal exit (not Normal/Shutdown)
//   - [Temporary]: Never restart; remove from supervisor on exit
//
// Example:
//
//	// Only restart if the task crashes, not if it completes normally
//	supervisor.NewChildSpec("task", taskStart,
//		supervisor.SetRestart(supervisor.Transient),
//	)
func SetRestart(restart Restart) ChildSpecOpt {
	return func(cs ChildSpec) ChildSpec {
		cs.Restart = restart
		return cs
	}
}

// SetShutdown configures how the child is terminated during supervisor shutdown
// or when restarted due to a sibling failure (in OneForAll/RestForOne strategies).
//
// The supervisor sends an exit signal and waits according to the [ShutdownOpt]:
//   - Timeout (default 5000ms): Wait up to N milliseconds, then kill
//   - BrutalKill: Kill immediately without waiting
//   - Infinity: Wait forever (recommended for supervisor children)
//
// Examples:
//
//	// Wait up to 10 seconds for graceful shutdown
//	supervisor.SetShutdown(supervisor.ShutdownOpt{Timeout: 10_000})
//
//	// Wait forever (for supervisor children)
//	supervisor.SetShutdown(supervisor.ShutdownOpt{Infinity: true})
//
//	// Kill immediately without waiting
//	supervisor.SetShutdown(supervisor.ShutdownOpt{BrutalKill: true})
func SetShutdown(shutdown ShutdownOpt) ChildSpecOpt {
	// TODO: do some validation to prevent nonsense shutdownopts
	return func(cs ChildSpec) ChildSpec {
		cs.Shutdown = shutdown
		return cs
	}
}

// SetChildType marks the child as a worker or supervisor.
// Default is [WorkerChild].
//
// Set to [SupervisorChild] when the child is another supervisor. This is
// primarily informational, but serves as documentation and may be used
// for introspection in the future.
//
// When setting SupervisorChild, you should also use SetShutdown with Infinity
// to allow the child supervisor's subtree to shut down gracefully:
//
//	supervisor.NewChildSpec("sub_supervisor", subSupStart,
//		supervisor.SetChildType(supervisor.SupervisorChild),
//		supervisor.SetShutdown(supervisor.ShutdownOpt{Infinity: true}),
//	)
func SetChildType(t ChildType) ChildSpecOpt {
	return func(cs ChildSpec) ChildSpec {
		cs.Type = t
		return cs
	}
}

// StartFunSpec is a function that starts a child process and links it to the supervisor.
//
// The function receives the supervisor's PID (sup) and MUST link the started process
// to it using [genserver.StartLink], [erl.SpawnLink], or similar linking functions.
// Failing to link means the supervisor won't receive exit signals from the child
// and cannot properly supervise it.
//
// Return values:
//   - (pid, nil): Child started successfully; pid is the child's PID
//   - (_, [exitreason.Ignore]): Child intentionally not started; supervisor continues
//     with remaining children. The child slot is marked as ignored.
//   - (_, error): Child failed to start; supervisor fails and stops all previously
//     started children (rollback behavior)
//
// Example using genserver:
//
//	func myWorkerStart(sup erl.PID) (erl.PID, error) {
//		return genserver.StartLink[MyState](sup, MyServer{}, initArgs)
//	}
//
// Example using raw process:
//
//	func myProcessStart(sup erl.PID) (erl.PID, error) {
//		pid := erl.SpawnLink(sup, MyRunnable{})
//		return pid, nil
//	}
//
// Example with conditional startup:
//
//	func conditionalStart(sup erl.PID) (erl.PID, error) {
//		if !config.FeatureEnabled {
//			return erl.PID{}, exitreason.Ignore // Skip this child
//		}
//		return genserver.StartLink[State](sup, Server{}, nil)
//	}
type StartFunSpec func(sup erl.PID) (erl.PID, error)

// NewChildSpec creates a child specification with the given ID and start function.
//
// The ID must be unique among siblings in the same supervisor. It is used for:
//   - Identifying the child in log messages and errors
//   - Determining restart scope in [RestForOne] strategy
//   - Future dynamic child management APIs
//
// The start function is called to spawn the child and must link it to the supervisor.
// See [StartFunSpec] for details on the function contract.
//
// Default values:
//   - Restart: [Permanent] (always restart)
//   - Shutdown: 5000ms timeout
//   - Type: [WorkerChild]
//
// Examples:
//
//	// Basic worker with defaults
//	spec := supervisor.NewChildSpec("my_worker", func(sup erl.PID) (erl.PID, error) {
//		return genserver.StartLink[State](sup, MyServer{}, nil)
//	})
//
//	// Transient worker that only restarts on crashes
//	spec := supervisor.NewChildSpec("task_runner", taskRunnerStart,
//		supervisor.SetRestart(supervisor.Transient),
//	)
//
//	// Worker with longer shutdown timeout
//	spec := supervisor.NewChildSpec("slow_worker", slowWorkerStart,
//		supervisor.SetShutdown(supervisor.ShutdownOpt{Timeout: 30_000}),
//	)
//
//	// Nested supervisor with infinite shutdown timeout
//	spec := supervisor.NewChildSpec("sub_supervisor", subSupStart,
//		supervisor.SetChildType(supervisor.SupervisorChild),
//		supervisor.SetShutdown(supervisor.ShutdownOpt{Infinity: true}),
//	)
func NewChildSpec(id string, start StartFunSpec, opts ...ChildSpecOpt) ChildSpec {
	cs := ChildSpec{
		ID:       id,
		Start:    start,
		Restart:  Permanent,
		Shutdown: ShutdownOpt{Timeout: 5_000},
	}

	for _, opt := range opts {
		cs = opt(cs)
	}
	return cs
}

// ChildSpec defines how a supervisor starts, monitors, and restarts a child process.
//
// Create using [NewChildSpec] with functional options rather than constructing directly.
// This ensures proper defaults are applied.
//
// The child's lifecycle is:
//  1. Supervisor calls Start function during initialization
//  2. Child runs and is monitored via link
//  3. When child exits, supervisor receives exit signal
//  4. Supervisor decides whether to restart based on Restart type and exit reason
//  5. During shutdown, supervisor terminates child according to Shutdown options
type ChildSpec struct {
	// ID uniquely identifies this child within its supervisor.
	// Must be non-empty and unique among siblings.
	//
	// Used in:
	//   - Log messages and error reporting
	//   - [RestForOne] strategy to determine which children to restart
	//   - Future dynamic child management APIs (start_child, terminate_child)
	ID string

	// Start is called to spawn the child process. The function receives the
	// supervisor's PID and MUST link the child to it (e.g., using [genserver.StartLink]
	// or [erl.SpawnLink]).
	//
	// Return (pid, nil) on success, (_, [exitreason.Ignore]) to skip this child,
	// or (_, err) to fail supervisor startup and trigger rollback.
	Start StartFunSpec

	// Restart determines when this child should be restarted after termination.
	// Default: [Permanent]
	//
	// See [Permanent], [Transient], and [Temporary] for behavior details.
	Restart Restart

	// Shutdown specifies how to terminate this child during supervisor shutdown
	// or when restarting due to strategy (OneForAll/RestForOne).
	// Default: 5000ms timeout
	//
	// See [ShutdownOpt] for available options (timeout, brutal kill, infinity).
	Shutdown ShutdownOpt

	// Type indicates whether this child is a worker or another supervisor.
	// Default: [WorkerChild]
	//
	// Set to [SupervisorChild] for nested supervisors. This is primarily
	// informational but signals that Infinity shutdown should be considered.
	Type ChildType

	// pid is the child's current PID after successful start.
	// Zero value ([erl.PID{}]) if not started or if Start returned Ignore.
	pid erl.PID

	// ignored is true if the child's Start function returned [exitreason.Ignore].
	// Ignored children are tracked but not running.
	ignored bool

	// terminated is true if the child has been stopped by the supervisor.
	// Used during shutdown to track completion.
	terminated bool
}

