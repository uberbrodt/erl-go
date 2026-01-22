package supervisor

import "github.com/uberbrodt/erl-go/erl"

// Strategy defines how a supervisor responds when a child process terminates.
// The strategy determines which children are restarted alongside the failed child.
//
// Choose a strategy based on the dependencies between children:
//   - [OneForOne]: Children are independent
//   - [OneForAll]: Children are tightly coupled; all must restart together
//   - [RestForOne]: Later children depend on earlier ones
type Strategy string

const (
	// OneForOne restarts only the terminated child process.
	// Other children are unaffected. This is the default strategy.
	//
	// Use when children are independent and don't rely on each other.
	//
	// Example scenario: Multiple independent worker processes where
	// the failure of one doesn't affect the others.
	OneForOne Strategy = "one_for_one"

	// OneForAll terminates all children and restarts them all (in start order)
	// when any single child terminates.
	//
	// Use when children are tightly coupled and cannot function correctly
	// if one of them fails.
	//
	// Example scenario: A database connection pool and cache that must be
	// in sync - if either fails, both should restart with fresh state.
	OneForAll Strategy = "one_for_all"

	// RestForOne terminates the failed child and all children that were started
	// after it (in start order), then restarts them in order.
	//
	// Use when children have dependencies on earlier siblings. Children are
	// started in the order they appear in ChildSpecs, so place dependencies
	// before dependents.
	//
	// Example scenario: A connection manager started first, followed by
	// worker processes that depend on it. If the connection manager fails,
	// all workers must also restart.
	RestForOne Strategy = "rest_for_one"
)

// Restart defines when a child process should be restarted after termination.
// This is set per-child in the [ChildSpec].
//
// The restart type interacts with the exit reason to determine behavior:
//   - [Permanent]: Always restarts regardless of reason
//   - [Transient]: Restarts only on abnormal exits (exceptions, kills)
//   - [Temporary]: Never restarts; removed from supervisor
type Restart string

const (
	// Permanent children are always restarted, regardless of exit reason.
	// This is the default restart type.
	//
	// Use for long-running services that should always be available,
	// such as HTTP servers, connection pools, or background workers.
	//
	// Exit reasons that trigger restart: all (Normal, Shutdown, Exception, etc.)
	Permanent Restart = "permanent"

	// Temporary children are never restarted and are removed from the supervisor
	// when they exit. They do not count toward restart intensity calculations.
	//
	// Use for one-off tasks, children managed by external logic, or processes
	// that should only run once.
	//
	// Exit reasons that trigger restart: none (always removed)
	Temporary Restart = "temporary"

	// Transient children are restarted only if they terminate abnormally.
	// An abnormal exit is any reason other than:
	//   - [exitreason.Normal]
	//   - [exitreason.Shutdown] (any variant)
	//   - [exitreason.SupervisorShutdown]
	//
	// Use for tasks that should complete successfully but should retry on failure.
	// For example, a data import job that should succeed eventually.
	//
	// Exit reasons that trigger restart: Exception, Kill, other errors
	// Exit reasons that do NOT restart: Normal, Shutdown, SupervisorShutdown
	Transient Restart = "transient"
)

// ShutdownOpt specifies how a child should be terminated during supervisor shutdown
// or when being restarted due to a sibling failure (in OneForAll/RestForOne strategies).
//
// The supervisor sends an exit signal to the child and then waits according to these options.
// Only one of BrutalKill, Timeout, or Infinity should be meaningfully set; they are
// evaluated in that priority order.
//
// Default behavior (zero value): 5000ms timeout.
//
// Examples:
//
//	// Wait up to 10 seconds for graceful shutdown
//	ShutdownOpt{Timeout: 10_000}
//
//	// Wait forever (for supervisor children)
//	ShutdownOpt{Infinity: true}
//
//	// Kill immediately without waiting
//	ShutdownOpt{BrutalKill: true}
type ShutdownOpt struct {
	// BrutalKill, if true, immediately kills the child by sending [exitreason.Kill]
	// without waiting for graceful shutdown. The child's Terminate callback may
	// not complete. Use sparingly, only when the child cannot be trusted to
	// shut down cleanly or when immediate termination is required.
	//
	// Takes precedence over Timeout and Infinity if set to true.
	BrutalKill bool

	// Timeout is the number of milliseconds to wait for the child to terminate
	// after sending [exitreason.SupervisorShutdown]. If the child doesn't exit
	// within this time, it is forcefully killed with [exitreason.Kill].
	//
	// Default is 5000 (5 seconds) when ShutdownOpt is zero value.
	// Set to 0 for no wait (equivalent to BrutalKill but sends shutdown first).
	//
	// Ignored if BrutalKill is true or Infinity is true.
	Timeout int

	// Infinity, if true, waits indefinitely for the child to terminate after
	// sending [exitreason.SupervisorShutdown].
	//
	// Recommended for supervisor children ([SupervisorChild]) to allow their
	// entire subtree to shut down gracefully. A supervisor may need significant
	// time to terminate all its children, especially with nested supervisors.
	//
	// Ignored if BrutalKill is true.
	Infinity bool
}

// ChildType indicates whether a child is a worker process or another supervisor.
// This is primarily informational and used for documentation/introspection purposes.
//
// When a child is a supervisor, consider:
//   - Setting [ShutdownOpt.Infinity] to allow the subtree to shut down gracefully
//   - Using appropriate restart intensity to handle cascading failures
type ChildType string

const (
	// SupervisorChild indicates the child is itself a supervisor managing its own children.
	//
	// When using this type, you should typically also set:
	//
	//	supervisor.SetShutdown(supervisor.ShutdownOpt{Infinity: true})
	//
	// This allows the child supervisor's entire subtree to shut down gracefully
	// rather than being forcefully killed after a timeout.
	SupervisorChild ChildType = "supervisor"

	// WorkerChild indicates the child is a worker process (not a supervisor).
	// This is the default child type.
	//
	// Workers are leaf nodes in the supervision tree - they do work but
	// don't supervise other processes.
	WorkerChild ChildType = "worker"
)

// ChildStatus represents the current state of a child process.
// Used in [ChildInfo] returned by [WhichChildren].
type ChildStatus string

const (
	// ChildRunning indicates the child process is currently running.
	ChildRunning ChildStatus = "running"

	// ChildTerminated indicates the child was stopped via [TerminateChild]
	// and its spec is retained for potential restart via [RestartChild].
	ChildTerminated ChildStatus = "terminated"

	// ChildUndefined indicates the child is not running because:
	//   - Its Start function returned [exitreason.Ignore]
	//   - It has not been started yet
	//   - It exited and was not restarted (Transient/Temporary with clean exit)
	ChildUndefined ChildStatus = "undefined"
)

// ChildInfo contains information about a single child process.
// Returned by [WhichChildren].
//
// This is equivalent to the tuples returned by Erlang's supervisor:which_children/1.
type ChildInfo struct {
	// ID is the unique identifier for this child within its supervisor.
	ID string

	// PID is the child's process ID if running, otherwise the zero value.
	// Check Status to determine if the child is actually running.
	PID erl.PID

	// Type indicates whether this child is a worker or supervisor.
	Type ChildType

	// Status indicates the child's current state (running, terminated, or undefined).
	Status ChildStatus

	// Restart is the child's restart strategy (Permanent, Transient, or Temporary).
	Restart Restart
}

// ChildCount contains counts of children by category.
// Returned by [CountChildren].
//
// This is equivalent to the proplist returned by Erlang's supervisor:count_children/1.
type ChildCount struct {
	// Specs is the total number of child specifications in the supervisor.
	// This includes running, terminated, and undefined children.
	Specs int

	// Active is the number of children currently running.
	Active int

	// Supervisors is the number of children with Type == [SupervisorChild].
	Supervisors int

	// Workers is the number of children with Type == [WorkerChild].
	Workers int
}
