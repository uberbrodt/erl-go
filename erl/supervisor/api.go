package supervisor

import (
	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/genserver"
	"github.com/uberbrodt/erl-go/erl/timeout"
)

type linkOpts struct {
	name erl.Name
}

type LinkOpts func(flags linkOpts) linkOpts

func SetName(name erl.Name) LinkOpts {
	return func(flags linkOpts) linkOpts {
		flags.name = name
		return flags
	}
}

// Instead of defining a callback module, just pass a list of children.
func StartDefaultLink(self erl.PID, children []ChildSpec, supFlags SupFlagsS, optFuns ...LinkOpts) (erl.PID, error) {
	ds := defaultSup{children: children, supflags: supFlags}
	return StartLink(self, ds, nil, optFuns...)
}

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
