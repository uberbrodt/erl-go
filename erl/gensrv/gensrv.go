package gensrv

import (
	"reflect"
	"time"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/genserver"
)

func (o *config[State]) SetName(name erl.Name) {
	o.name = name
}

func (o *config[State]) GetName() erl.Name {
	return o.name
}

func (o *config[State]) SetStartTimeout(tout time.Duration) {
	o.startTimeout = tout
}

func (o *config[State]) GetStartTimeout() time.Duration {
	return o.startTimeout
}

type CastHandle[State any] struct {
	Arg     any
	Handler func(self erl.PID, request any, state State) (newState State, continueTerm any, err error)
}

type CallHandle[State any] struct {
	Arg     any
	Handler func(self erl.PID, request any, from genserver.From, state State) (result genserver.CallResult[State], err error)
}

type GenSrvOpt[State any] func(c *config[State])

func Start[State any](self erl.PID, arg any, opts ...GenSrvOpt[State]) (erl.PID, error) {
	conf := doConf[State](opts...)

	return genserver.Start[State](self, &CB[State]{conf: conf}, arg, genserver.InheritOpts(conf))
}

func StartLink[State any](self erl.PID, arg any, opts ...GenSrvOpt[State]) (erl.PID, error) {
	conf := doConf[State](opts...)
	return genserver.StartLink[State](self, &CB[State]{conf: conf}, arg, genserver.InheritOpts(conf))
}

func StartMonitor[State any](self erl.PID, arg any, opts ...GenSrvOpt[State]) (erl.PID, erl.Ref, error) {
	conf := doConf[State](opts...)
	return genserver.StartMonitor[State](self, &CB[State]{conf: conf}, arg, genserver.InheritOpts(conf))
}

func doConf[State any](opts ...GenSrvOpt[State]) *config[State] {
	conf := &config[State]{
		castFuns:     make(map[reflect.Type]func(self erl.PID, arg any, state State) (newState State, continu any, err error)),
		callFuns:     make(map[reflect.Type]func(self erl.PID, request any, from genserver.From, state State) (genserver.CallResult[State], error)),
		infoFuns:     make(map[reflect.Type]func(self erl.PID, arg any, state State) (newState State, continu any, err error)),
		continueFuns: make(map[reflect.Type]func(self erl.PID, contTerm any, state State) (newState State, continu any, err error)),
	}

	for _, opt := range opts {
		opt(conf)
	}
	return conf
}

func RegisterInit[State any](init func(self erl.PID, a any) (genserver.InitResult[State], error)) GenSrvOpt[State] {
	return func(c *config[State]) {
		c.initFun = init
	}
}

func RegisterCast[State any](matchType any, fn func(self erl.PID, a any, state State) (newState State, continu any, err error)) GenSrvOpt[State] {
	return func(c *config[State]) {
		termT := reflect.TypeOf(matchType)
		c.castFuns[termT] = fn
	}
}

func RegisterInfo[State any](matchType any, fn func(self erl.PID, a any, state State) (newState State, continu any, err error)) GenSrvOpt[State] {
	return func(c *config[State]) {
		termT := reflect.TypeOf(matchType)
		c.infoFuns[termT] = fn
	}
}

func RegisterCall[State any](matchType any, fn func(self erl.PID, request any, from genserver.From, state State) (result genserver.CallResult[State], err error)) GenSrvOpt[State] {
	return func(c *config[State]) {
		termT := reflect.TypeOf(matchType)
		c.callFuns[termT] = fn
	}
}

func RegisterContinue[State any](matchType any, fn func(self erl.PID, cont any, state State) (newState State, continu any, err error)) GenSrvOpt[State] {
	return func(c *config[State]) {
		termT := reflect.TypeOf(matchType)
		c.continueFuns[termT] = fn
	}
}

func RegisterTerminate[State any](terminate func(self erl.PID, reason error, state State)) GenSrvOpt[State] {
	return func(c *config[State]) {
		c.terminateFun = terminate
	}
}
