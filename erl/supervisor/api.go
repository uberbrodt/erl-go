package supervisor

import (
	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/genserver"
	"github.com/uberbrodt/erl-go/erl/timeout"
)

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

