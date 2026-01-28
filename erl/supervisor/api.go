package supervisor

import (
	"fmt"
	"time"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/genserver"
	"github.com/uberbrodt/erl-go/erl/timeout"
)

// DefaultAPITimeout is the default timeout for supervisor API calls.
// Individual calls can use the *Timeout variants for custom timeouts.
var DefaultAPITimeout = timeout.Default

type linkOpts struct {
	name erl.Name
}

// LinkOpts is a functional option for configuring supervisor startup.
// Use with [StartDefaultLink] or [StartLink].
type LinkOpts func(flags linkOpts) linkOpts

// SetName registers the supervisor under the given name, allowing it to be
// looked up via [erl.WhereIs] or addressed by name instead of PID.
//
// Named supervisors are useful for:
//   - Looking up the supervisor from anywhere in the application
//   - Addressing the supervisor without passing the PID explicitly
//   - Debugging and introspection
//
// Example:
//
//	supPID, err := supervisor.StartDefaultLink(self, children, flags,
//		supervisor.SetName("my_supervisor"))
//
//	// Later, look up by name:
//	pid := erl.WhereIs("my_supervisor")
func SetName(name erl.Name) LinkOpts {
	return func(flags linkOpts) linkOpts {
		flags.name = name
		return flags
	}
}

// StartDefaultLink starts a supervisor with a static list of children.
//
// This is the simplest way to create a supervisor when the children are known
// at compile time. For dynamic child configuration based on runtime arguments,
// use [StartLink] with a custom [Supervisor] callback.
//
// The supervisor is linked to the calling process (self), meaning:
//   - If the supervisor terminates abnormally, self receives an exit signal
//   - If self has TrapExit enabled, it receives an [erl.ExitMsg] instead
//   - If self terminates, the supervisor receives an exit signal
//
// Children are started in slice order. If any child fails to start (returns an
// error other than [exitreason.Ignore]), all previously started children are
// stopped (rollback) and StartDefaultLink returns an error.
//
// Parameters:
//   - self: The calling process's PID (supervisor will be linked to this)
//   - children: Child specifications in start order
//   - supFlags: Supervisor configuration (strategy, intensity, period)
//   - optFuns: Optional configuration (e.g., [SetName] for registration)
//
// Returns:
//   - (pid, nil): Supervisor started successfully
//   - (_, error): Startup failed due to:
//   - Duplicate child IDs
//   - Child failed to start
//   - Callback returned Ignore=true
//
// Example:
//
//	children := []supervisor.ChildSpec{
//		supervisor.NewChildSpec("database", dbStart),
//		supervisor.NewChildSpec("cache", cacheStart,
//			supervisor.SetRestart(supervisor.Transient)),
//		supervisor.NewChildSpec("worker", workerStart),
//	}
//
//	supFlags := supervisor.NewSupFlags(
//		supervisor.SetStrategy(supervisor.RestForOne),
//		supervisor.SetIntensity(5),
//		supervisor.SetPeriod(60),
//	)
//
//	supPID, err := supervisor.StartDefaultLink(self, children, supFlags)
//	if err != nil {
//		return fmt.Errorf("failed to start supervisor: %w", err)
//	}
func StartDefaultLink(self erl.PID, children []ChildSpec, supFlags SupFlagsS, optFuns ...LinkOpts) (erl.PID, error) {
	ds := defaultSup{children: children, supflags: supFlags}
	return StartLink(self, ds, nil, optFuns...)
}

// StartLink starts a supervisor with a custom callback module.
//
// Use this when children need to be determined dynamically based on runtime
// arguments, or when you need custom initialization logic. For static child
// lists known at compile time, [StartDefaultLink] is simpler.
//
// The callback's [Supervisor.Init] method is invoked with the provided args
// to obtain the [ChildSpec] list and [SupFlagsS].
//
// The supervisor is linked to the calling process (self), meaning:
//   - If the supervisor terminates abnormally, self receives an exit signal
//   - If self has TrapExit enabled, it receives an [erl.ExitMsg] instead
//   - If self terminates, the supervisor receives an exit signal
//
// Parameters:
//   - self: The calling process's PID (supervisor will be linked to this)
//   - callback: Implementation of [Supervisor] interface
//   - args: Arguments passed to callback's Init method
//   - optFuns: Optional configuration (e.g., [SetName] for registration)
//
// Returns:
//   - (pid, nil): Supervisor started successfully
//   - (_, error): Startup failed (see [StartDefaultLink] for error cases)
//
// Example:
//
//	type MySupervisor struct{}
//
//	func (s MySupervisor) Init(self erl.PID, args any) supervisor.InitResult {
//		config := args.(AppConfig)
//
//		// Create children based on configuration
//		children := make([]supervisor.ChildSpec, 0)
//		children = append(children, supervisor.NewChildSpec("conn_manager",
//			func(sup erl.PID) (erl.PID, error) {
//				return genserver.StartLink[ConnState](sup, ConnManager{}, config.DBUrl)
//			},
//		))
//
//		// Add workers based on config
//		for i := 0; i < config.WorkerCount; i++ {
//			id := fmt.Sprintf("worker_%d", i)
//			children = append(children, supervisor.NewChildSpec(id, workerStartFn))
//		}
//
//		return supervisor.InitResult{
//			SupFlags:   supervisor.NewSupFlags(supervisor.SetStrategy(supervisor.RestForOne)),
//			ChildSpecs: children,
//		}
//	}
//
//	// Start the supervisor
//	supPID, err := supervisor.StartLink(self, MySupervisor{}, appConfig,
//		supervisor.SetName("app_supervisor"))
func StartLink(self erl.PID, callback Supervisor, args any, optFuns ...LinkOpts) (erl.PID, error) {
	opts := linkOpts{}

	for _, fn := range optFuns {
		opts = fn(opts)
	}

	gsOpts := make([]genserver.StartOpt, 0)

	if opts.name != "" {
		gsOpts = append(gsOpts, genserver.SetName(opts.name))
	}

	gsOpts = append(gsOpts, genserver.SetStartTimeout(timeout.Infinity))

	sup := SupervisorS{
		callback: callback,
	}

	return genserver.StartLink[supervisorState](self, sup, args, gsOpts...)
}

// =============================================================================
// Dynamic Child Management APIs
// =============================================================================

// WhichChildren returns information about all children managed by the supervisor.
//
// For each child, returns:
//   - ID: The child's unique identifier
//   - PID: The child's process ID (zero if not running)
//   - Type: Whether the child is a worker or supervisor
//   - Status: Current state (running, terminated, undefined)
//   - Restart: The child's restart strategy
//
// This is equivalent to Erlang's supervisor:which_children/1.
//
// Example:
//
//	children, err := supervisor.WhichChildren(supPID)
//	if err != nil {
//		return err
//	}
//	for _, child := range children {
//		fmt.Printf("Child %s: status=%s, pid=%v\n", child.ID, child.Status, child.PID)
//	}
func WhichChildren(sup erl.Dest) ([]ChildInfo, error) {
	return WhichChildrenTimeout(sup, DefaultAPITimeout)
}

// WhichChildrenTimeout is like [WhichChildren] but with a custom timeout.
func WhichChildrenTimeout(sup erl.Dest, tout time.Duration) ([]ChildInfo, error) {
	self := erl.RootPID()

	result, err := genserver.Call(self, sup, whichChildrenRequest{}, tout)
	if err != nil {
		return nil, err
	}

	resp, ok := result.(whichChildrenResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected response type: %T", result)
	}

	return resp.children, nil
}

// CountChildren returns counts of children by category.
//
// Returns:
//   - Specs: Total number of child specifications
//   - Active: Number of currently running children
//   - Supervisors: Number of children with SupervisorChild type
//   - Workers: Number of children with WorkerChild type
//
// This is equivalent to Erlang's supervisor:count_children/1.
//
// Example:
//
//	count, err := supervisor.CountChildren(supPID)
//	if err != nil {
//		return err
//	}
//	fmt.Printf("Total: %d, Active: %d\n", count.Specs, count.Active)
func CountChildren(sup erl.Dest) (ChildCount, error) {
	return CountChildrenTimeout(sup, DefaultAPITimeout)
}

// CountChildrenTimeout is like [CountChildren] but with a custom timeout.
func CountChildrenTimeout(sup erl.Dest, tout time.Duration) (ChildCount, error) {
	self := erl.RootPID()

	result, err := genserver.Call(self, sup, countChildrenRequest{}, tout)
	if err != nil {
		return ChildCount{}, err
	}

	resp, ok := result.(countChildrenResponse)
	if !ok {
		return ChildCount{}, fmt.Errorf("unexpected response type: %T", result)
	}

	return resp.count, nil
}

// StartChild dynamically adds a child specification and starts the child.
//
// Returns the child's PID on success. Possible errors:
//   - [ErrAlreadyStarted]: A child with this ID exists and is running
//     (use [errors.As] to get [AlreadyStartedError] with the existing PID)
//   - [ErrAlreadyPresent]: A child with this ID exists but is not running
//     (use [RestartChild] to restart it, or [DeleteChild] then StartChild)
//   - Other errors: The child failed to start
//
// Note: StartChild does NOT affect restart intensity calculations. Only
// automatic restarts triggered by child exits count toward intensity.
//
// This is equivalent to Erlang's supervisor:start_child/2.
//
// Example:
//
//	spec := supervisor.NewChildSpec("new_worker", workerStartFn,
//		supervisor.SetRestart(supervisor.Transient))
//	pid, err := supervisor.StartChild(supPID, spec)
//	if errors.Is(err, supervisor.ErrAlreadyStarted) {
//		// Child already exists and is running
//	}
func StartChild(sup erl.Dest, spec ChildSpec) (erl.PID, error) {
	return StartChildTimeout(sup, spec, DefaultAPITimeout)
}

// StartChildTimeout is like [StartChild] but with a custom timeout.
func StartChildTimeout(sup erl.Dest, spec ChildSpec, tout time.Duration) (erl.PID, error) {
	self := erl.RootPID()

	result, err := genserver.Call(self, sup, startChildRequest{spec: spec}, tout)
	if err != nil {
		return erl.PID{}, err
	}

	resp, ok := result.(startChildResponse)
	if !ok {
		return erl.PID{}, fmt.Errorf("unexpected response type: %T", result)
	}

	return resp.pid, resp.err
}

// TerminateChild stops a child process but keeps its specification.
//
// The child can be restarted later using [RestartChild]. The specification
// can be removed using [DeleteChild].
//
// This operation does NOT count toward restart intensity.
//
// Returns:
//   - nil: Child was terminated (or was already not running)
//   - [ErrNotFound]: No child with the given ID exists
//
// This is equivalent to Erlang's supervisor:terminate_child/2.
//
// Example:
//
//	// Stop the child but keep its spec for later restart
//	err := supervisor.TerminateChild(supPID, "my_worker")
//	if err != nil {
//		return err
//	}
//	// Later, restart it
//	pid, err := supervisor.RestartChild(supPID, "my_worker")
func TerminateChild(sup erl.Dest, childID string) error {
	return TerminateChildTimeout(sup, childID, DefaultAPITimeout)
}

// TerminateChildTimeout is like [TerminateChild] but with a custom timeout.
func TerminateChildTimeout(sup erl.Dest, childID string, tout time.Duration) error {
	self := erl.RootPID()

	result, err := genserver.Call(self, sup, terminateChildRequest{childID: childID}, tout)
	if err != nil {
		return err
	}

	resp, ok := result.(terminateChildResponse)
	if !ok {
		return fmt.Errorf("unexpected response type: %T", result)
	}

	return resp.err
}

// RestartChild restarts a child that was previously terminated.
//
// The child must have been terminated via [TerminateChild] or exited
// on its own (for Transient/Temporary children that don't auto-restart).
//
// Returns:
//   - (pid, nil): Child was restarted successfully
//   - (_, [ErrNotFound]): No child with the given ID exists
//   - (_, [ErrRunning]): Child is already running
//   - (_, error): Child failed to start
//
// This is equivalent to Erlang's supervisor:restart_child/2.
//
// Example:
//
//	pid, err := supervisor.RestartChild(supPID, "my_worker")
//	if errors.Is(err, supervisor.ErrRunning) {
//		// Child is already running, get its PID from WhichChildren
//	}
func RestartChild(sup erl.Dest, childID string) (erl.PID, error) {
	return RestartChildTimeout(sup, childID, DefaultAPITimeout)
}

// RestartChildTimeout is like [RestartChild] but with a custom timeout.
func RestartChildTimeout(sup erl.Dest, childID string, tout time.Duration) (erl.PID, error) {
	self := erl.RootPID()

	result, err := genserver.Call(self, sup, restartChildRequest{childID: childID}, tout)
	if err != nil {
		return erl.PID{}, err
	}

	resp, ok := result.(restartChildResponse)
	if !ok {
		return erl.PID{}, fmt.Errorf("unexpected response type: %T", result)
	}

	return resp.pid, resp.err
}

// DeleteChild removes a child specification from the supervisor.
//
// The child must not be running. Use [TerminateChild] first if needed.
//
// Returns:
//   - nil: Child specification was removed
//   - [ErrNotFound]: No child with the given ID exists
//   - [ErrRunning]: Child is still running (terminate first)
//
// This is equivalent to Erlang's supervisor:delete_child/2.
//
// Example:
//
//	// First terminate the child
//	err := supervisor.TerminateChild(supPID, "my_worker")
//	if err != nil {
//		return err
//	}
//	// Then delete its spec
//	err = supervisor.DeleteChild(supPID, "my_worker")
func DeleteChild(sup erl.Dest, childID string) error {
	return DeleteChildTimeout(sup, childID, DefaultAPITimeout)
}

// DeleteChildTimeout is like [DeleteChild] but with a custom timeout.
func DeleteChildTimeout(sup erl.Dest, childID string, tout time.Duration) error {
	self := erl.RootPID()

	result, err := genserver.Call(self, sup, deleteChildRequest{childID: childID}, tout)
	if err != nil {
		return err
	}

	resp, ok := result.(deleteChildResponse)
	if !ok {
		return fmt.Errorf("unexpected response type: %T", result)
	}

	return resp.err
}
