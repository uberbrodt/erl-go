package supervisor

import "github.com/uberbrodt/erl-go/erl"

type ChildSpecOpt func(cs ChildSpec) ChildSpec

func SetRestart(restart Restart) ChildSpecOpt {
	return func(cs ChildSpec) ChildSpec {
		cs.Restart = restart
		return cs
	}
}

func SetShutdown(shutdown ShutdownOpt) ChildSpecOpt {
	// TODO: do some validation to prevent nonsense shutdownopts
	return func(cs ChildSpec) ChildSpec {
		cs.Shutdown = shutdown
		return cs
	}
}

func SetChildType(t ChildType) ChildSpecOpt {
	return func(cs ChildSpec) ChildSpec {
		cs.Type = t
		return cs
	}
}

type StartFunSpec func(sup erl.PID) (erl.PID, error)

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

type ChildSpec struct {
	// used to identify the child process internally by the Supervisor
	ID string
	// error's of type exitreason.S are special, such as [exitreason.Ignore]. The function must link the process
	// to the supervisor
	Start      StartFunSpec
	Restart    Restart
	Shutdown   ShutdownOpt
	Type       ChildType
	pid        erl.PID
	ignored    bool
	terminated bool
}
