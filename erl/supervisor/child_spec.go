package supervisor

import "github.com/uberbrodt/erl-go/erl"

// ChildSpecOpt is a functional option for configuring a [ChildSpec].
type ChildSpecOpt func(cs ChildSpec) ChildSpec

// SetRestart sets when the child should be restarted after termination.
// Default is [Permanent] (always restart).
//
// See [Permanent], [Transient], and [Temporary] for available options.
func SetRestart(restart Restart) ChildSpecOpt {
	return func(cs ChildSpec) ChildSpec {
		cs.Restart = restart
		return cs
	}
}

// SetShutdown configures how the child is terminated during supervisor shutdown
// or when restarted due to a sibling failure.
//
// Default is a 5000ms timeout. For supervisor children, consider using
// ShutdownOpt{Infinity: true} to allow the subtree to shut down gracefully.
//
// Example:
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
// Set to [SupervisorChild] for supervisors. This is primarily informational,
// but when set to SupervisorChild, consider also using SetShutdown with Infinity
// to allow the child's subtree to shut down gracefully.
func SetChildType(t ChildType) ChildSpecOpt {
	return func(cs ChildSpec) ChildSpec {
		cs.Type = t
		return cs
	}
}

// StartFunSpec is a function that starts a child process and links it to the supervisor.
//
// The function receives the supervisor's PID (sup) and MUST link the started process
// to it. Use [genserver.StartLink], [erl.SpawnLink], or similar linking functions.
// Failing to link will cause the supervisor to not receive exit signals from the child.
//
// Return values:
//   - (pid, nil): Child started successfully; pid is the child's PID
//   - (_, exitreason.Ignore): Child intentionally not started; supervisor continues
//   - (_, error): Child failed to start; supervisor fails (or rolls back all children)
//
// Example:
//
//	func myChildStart(sup erl.PID) (erl.PID, error) {
//		return genserver.StartLink[MyState](sup, MyServer{}, initArgs)
//	}
type StartFunSpec func(sup erl.PID) (erl.PID, error)

// NewChildSpec creates a child specification with the given ID and start function.
//
// The ID must be unique among siblings in the same supervisor. It is used for
// identifying the child in logs, errors, and for the [RestForOne] strategy.
//
// The start function is called to spawn the child and must link it to the supervisor.
// See [StartFunSpec] for details.
//
// Default values:
//   - Restart: [Permanent] (always restart)
//   - Shutdown: 5000ms timeout
//   - Type: [WorkerChild]
//
// Example:
//
//	// Basic worker with defaults
//	spec := supervisor.NewChildSpec("my_worker", func(sup erl.PID) (erl.PID, error) {
//		return genserver.StartLink[State](sup, MyServer{}, nil)
//	})
//
//	// Transient worker that only restarts on crashes
//	spec := supervisor.NewChildSpec("task_runner",
//		taskRunnerStart,
//		supervisor.SetRestart(supervisor.Transient),
//	)
//
//	// Nested supervisor with infinite shutdown timeout
//	spec := supervisor.NewChildSpec("sub_supervisor",
//		subSupStart,
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
// Create using [NewChildSpec] with functional options.
type ChildSpec struct {
	// ID uniquely identifies this child within its supervisor.
	// Used in log messages, error reporting, and the [RestForOne] strategy
	// to determine which children to restart.
	ID string

	// Start is called to spawn the child process. The function receives the
	// supervisor's PID and MUST link the child to it (e.g., using genserver.StartLink
	// or erl.SpawnLink).
	//
	// Return (pid, nil) on success, (_, exitreason.Ignore) to skip this child,
	// or (_, err) to fail supervisor startup.
	Start StartFunSpec

	// Restart determines when this child should be restarted after termination.
	// See [Permanent], [Transient], and [Temporary].
	Restart Restart

	// Shutdown specifies how to terminate this child during supervisor shutdown
	// or restart. See [ShutdownOpt] for options (timeout, brutal kill, or infinity).
	Shutdown ShutdownOpt

	// Type indicates whether this child is a worker or another supervisor.
	// Primarily informational; set to [SupervisorChild] for nested supervisors
	// and consider using Infinity shutdown for those.
	Type ChildType

	// pid is the child's current PID (set after successful start)
	pid erl.PID
	// ignored is true if the child's Start returned exitreason.Ignore
	ignored bool
	// terminated is true if the child has been stopped
	terminated bool
}
