package supervisor

// Strategy defines how a supervisor responds when a child process terminates.
// The strategy determines which children are restarted alongside the failed child.
type Strategy string

const (
	// OneForOne restarts only the terminated child process.
	// Other children are unaffected. This is the default strategy.
	// Use when children are independent and don't rely on each other.
	OneForOne Strategy = "one_for_one"

	// OneForAll terminates all children and restarts them all (in start order)
	// when any single child terminates. Use when children are tightly coupled
	// and cannot function correctly if one of them fails.
	OneForAll Strategy = "one_for_all"

	// RestForOne terminates the failed child and all children that were started
	// after it (in start order), then restarts them in order. Use when children
	// have dependencies on earlier siblings (e.g., a connection pool that later
	// workers depend on).
	RestForOne Strategy = "rest_for_one"
)

// Restart defines when a child process should be restarted after termination.
// This is set per-child in the [ChildSpec].
type Restart string

const (
	// Permanent children are always restarted, regardless of exit reason.
	// Use for long-running services that should always be available.
	// This is the default restart type.
	Permanent Restart = "permanent"

	// Temporary children are never restarted and are removed from the supervisor
	// when they exit. They do not count toward restart intensity calculations.
	// Use for one-off tasks or children managed by external logic.
	Temporary Restart = "temporary"

	// Transient children are restarted only if they terminate abnormally.
	// An abnormal exit is any reason other than Normal, Shutdown, or SupervisorShutdown.
	// Use for tasks that should complete successfully but retry on failure.
	Transient Restart = "transient"
)

// ShutdownOpt specifies how a child should be terminated during supervisor shutdown
// or when being restarted due to a sibling failure (in OneForAll/RestForOne strategies).
//
// The supervisor sends an exit signal to the child and then waits according to these options.
// Only one of BrutalKill, Timeout, or Infinity should be meaningfully set.
type ShutdownOpt struct {
	// BrutalKill, if true, immediately kills the child without waiting for graceful shutdown.
	// The child's Terminate callback may not complete. Use sparingly.
	// Takes precedence over Timeout and Infinity.
	BrutalKill bool

	// Timeout is the number of milliseconds to wait for the child to terminate
	// after sending an exit signal. If the child doesn't exit within this time,
	// it is forcefully killed. Default is 5000 (5 seconds).
	// Ignored if BrutalKill is true or Infinity is true.
	Timeout int

	// Infinity, if true, waits indefinitely for the child to terminate.
	// Recommended for supervisor children to allow their entire subtree to
	// shut down gracefully. Ignored if BrutalKill is true.
	Infinity bool
}

// ChildType indicates whether a child is a worker process or another supervisor.
// This is primarily informational and used for documentation/introspection purposes.
type ChildType string

const (
	// SupervisorChild indicates the child is itself a supervisor managing its own children.
	// When using this type, consider setting ShutdownOpt{Infinity: true} to allow the
	// child supervisor's entire subtree to shut down gracefully.
	SupervisorChild ChildType = "supervisor"

	// WorkerChild indicates the child is a worker process (not a supervisor).
	// This is the default child type.
	WorkerChild ChildType = "worker"
)
