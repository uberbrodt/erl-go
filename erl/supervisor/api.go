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
type LinkOpts func(flags linkOpts) linkOpts

// SetName registers the supervisor under the given name, allowing it to be
// looked up via [erl.WhereIs] or addressed by name instead of PID.
//
// Example:
//
//	supPID, err := supervisor.StartDefaultLink(self, children, flags,
//		supervisor.SetName("my_supervisor"))
//	// Later:
//	pid := erl.WhereIs("my_supervisor")
func SetName(name erl.Name) LinkOpts {
	return func(flags linkOpts) linkOpts {
		flags.name = name
		return flags
	}
}

// StartDefaultLink starts a supervisor with a static list of children.
// This is the simplest way to create a supervisor when the children are known at compile time.
//
// The supervisor is linked to the calling process (self), meaning if the supervisor
// terminates abnormally, self will receive an exit signal (or an [erl.ExitMsg] if
// self has TrapExit enabled).
//
// Children are started in the order they appear in the slice. If any child fails to start,
// all previously started children are stopped and an error is returned.
//
// Returns the supervisor's PID on success. Returns an error if:
//   - A child spec has a duplicate ID
//   - Any child fails to start (and doesn't return exitreason.Ignore)
//   - The supervisor callback returns Ignore=true
//
// Example:
//
//	children := []supervisor.ChildSpec{
//		supervisor.NewChildSpec("worker", func(sup erl.PID) (erl.PID, error) {
//			return genserver.StartLink[State](sup, MyServer{}, nil)
//		}),
//	}
//	supFlags := supervisor.NewSupFlags(supervisor.SetStrategy(supervisor.OneForOne))
//	supPID, err := supervisor.StartDefaultLink(self, children, supFlags)
func StartDefaultLink(self erl.PID, children []ChildSpec, supFlags SupFlagsS, optFuns ...LinkOpts) (erl.PID, error) {
	ds := defaultSup{children: children, supflags: supFlags}
	return StartLink(self, ds, nil, optFuns...)
}

// StartLink starts a supervisor with a custom callback module.
// Use this when children need to be determined dynamically based on runtime arguments.
//
// The callback's Init method is invoked with the provided args to obtain the
// [ChildSpec] list and [SupFlagsS]. The supervisor is linked to the calling process (self).
//
// This is useful when:
//   - Children depend on configuration loaded at runtime
//   - The number or type of children varies based on arguments
//   - You need custom initialization logic before defining children
//
// Example:
//
//	type MySupervisor struct{}
//
//	func (s MySupervisor) Init(self erl.PID, args any) supervisor.InitResult {
//		config := args.(MyConfig)
//		children := make([]supervisor.ChildSpec, config.WorkerCount)
//		for i := range children {
//			id := fmt.Sprintf("worker_%d", i)
//			children[i] = supervisor.NewChildSpec(id, workerStartFn)
//		}
//		return supervisor.InitResult{
//			SupFlags:   supervisor.NewSupFlags(),
//			ChildSpecs: children,
//		}
//	}
//
//	supPID, err := supervisor.StartLink(self, MySupervisor{}, myConfig)
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

