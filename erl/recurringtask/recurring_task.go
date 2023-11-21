package recurringtask

import (
	"time"

	"github.com/uberbrodt/erl-go/chronos"
	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/genserver"
)

type taskOpts struct {
	name         erl.Name
	startTimeout time.Duration
	loopTimeout  time.Duration
}

func (o *taskOpts) SetName(name erl.Name) {
	o.name = name
}

func (o *taskOpts) GetName() erl.Name {
	return o.name
}

func (o *taskOpts) SetStartTimeout(tout time.Duration) {
	o.startTimeout = tout
}

func (o *taskOpts) GetStartTimeout() time.Duration {
	return o.startTimeout
}

type StartOpt func(opts *taskOpts) *taskOpts

// Register this task with a name
func SetName(name erl.Name) StartOpt {
	return func(opts *taskOpts) *taskOpts {
		opts.name = name
		return opts
	}
}

// How often the task function should run. It is inclusive of task run time.
// so if the interval is 5m and the task takes 1m to run, the task will
// run effectively every 6m
func SetInterval(interval time.Duration) StartOpt {
	return func(opts *taskOpts) *taskOpts {
		opts.loopTimeout = interval
		return opts
	}
}

// How long to wait for this process to start before an error is returned
func SetStartTimeout(tout time.Duration) StartOpt {
	return func(opts *taskOpts) *taskOpts {
		opts.startTimeout = tout
		return opts
	}
}

func defaultTaskOpts() *taskOpts {
	return &taskOpts{loopTimeout: chronos.Dur("5m")}
}

func Start[S any, A any](self erl.PID, taskFun func(self erl.PID, state S) (S, error), initFun func(self erl.PID, args A) (S, error), args A, opts ...StartOpt) (erl.PID, error) {
	topts := defaultTaskOpts()
	for _, opt := range opts {
		topts = opt(topts)
	}

	c := rtConfig[S, A]{taskFun: taskFun, initFun: initFun, taskArgs: args, loopTimeout: topts.loopTimeout}
	return genserver.Start[rtState[S, A]](self, &rtSrv[S, A]{}, c, genserver.InheritOpts(topts))
}

func StartLink[S any, A any](self erl.PID, taskFun func(self erl.PID, state S) (S, error), initFun func(self erl.PID, args A) (S, error), args A, opts ...StartOpt) (erl.PID, error) {
	topts := defaultTaskOpts()
	for _, opt := range opts {
		topts = opt(topts)
	}

	c := rtConfig[S, A]{taskFun: taskFun, initFun: initFun, taskArgs: args, loopTimeout: topts.loopTimeout}
	return genserver.StartLink[rtState[S, A]](self, &rtSrv[S, A]{}, c, genserver.InheritOpts(topts))
}

func StartMonitor[S any, A any](self erl.PID, taskFun func(self erl.PID, state S) (S, error), initFun func(self erl.PID, args A) (S, error), args A, opts ...StartOpt) (erl.PID, erl.Ref, error) {
	topts := defaultTaskOpts()
	for _, opt := range opts {
		topts = opt(topts)
	}
	c := rtConfig[S, A]{taskFun: taskFun, initFun: initFun, taskArgs: args, loopTimeout: topts.loopTimeout}
	return genserver.StartMonitor[rtState[S, A]](self, &rtSrv[S, A]{}, c, genserver.InheritOpts(topts))
}

func Stop(self erl.PID, task erl.PID, opts ...genserver.ExitOpt) error {
	return genserver.Stop(self, task, opts...)
}
