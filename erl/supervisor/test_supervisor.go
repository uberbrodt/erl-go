package supervisor

import (
	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/genserver"
)

func NewTestServerChildSpec[STATE any](id string, ts genserver.TestServer[STATE], gsOpts genserver.StartOpts, opts ...ChildSpecOpt) ChildSpec {
	return NewChildSpec(id, func(sup erl.PID) (erl.PID, error) {
		return genserver.StartLink[STATE](sup, ts, nil, genserver.InheritOpts(gsOpts))
	})
}
