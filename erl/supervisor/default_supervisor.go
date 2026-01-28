package supervisor

import "github.com/uberbrodt/erl-go/erl"

// defaultSup is an internal Supervisor implementation used by StartDefaultLink.
// It simply returns the provided children and flags without any dynamic logic.
type defaultSup struct {
	children []ChildSpec
	supflags SupFlagsS
}

// Init implements Supervisor.Init by returning the static configuration
// provided to StartDefaultLink.
func (ds defaultSup) Init(self erl.PID, args any) InitResult {
	return InitResult{
		SupFlags:   ds.supflags,
		ChildSpecs: ds.children,
	}
}
