package genserver

import (
	"time"

	"github.com/uberbrodt/erl-go/chronos"
	"github.com/uberbrodt/erl-go/erl"
)

type genSrvOpts struct {
	name         erl.Name
	startTimeout time.Duration
}

type StartOpts interface {
	SetName(erl.Name)
	GetName() erl.Name
	SetStartTimeout(time.Duration)
	GetStartTimeout() time.Duration
}

func (o *genSrvOpts) SetName(name erl.Name) {
	o.name = name
}

func (o *genSrvOpts) GetName() erl.Name {
	return o.name
}

func (o *genSrvOpts) SetStartTimeout(tout time.Duration) {
	o.startTimeout = tout
}

func (o *genSrvOpts) GetStartTimeout() time.Duration {
	return o.startTimeout
}

func InheritOpts(o StartOpts) StartOpt {
	return func(opts StartOpts) StartOpts {
		if o.GetName() != "" {
			opts.SetName(o.GetName())
		}

		if o.GetStartTimeout() != 0 {
			opts.SetStartTimeout(o.GetStartTimeout())
		}
		return opts
	}
}

type StartOpt func(opts StartOpts) StartOpts

func DefaultOpts() StartOpts {
	return &genSrvOpts{startTimeout: chronos.Dur("5s")}
}

func SetName(name erl.Name) StartOpt {
	return func(opts StartOpts) StartOpts {
		opts.SetName(name)
		return opts
	}
}
