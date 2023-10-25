package supervisor

import "github.com/uberbrodt/erl-go/erl"

type defaultSup struct {
	children []ChildSpec
	supflags SupFlagsS
}

func (ds defaultSup) Init(self erl.PID, args any) InitResult {
	return InitResult{
		SupFlags:   ds.supflags,
		ChildSpecs: ds.children,
	}
}
