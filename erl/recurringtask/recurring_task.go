// A recurringtask is similar to a cronjob; it is designed to be run on a soft schedule dictated by the
// [SetInterval] option. However, there are important differences with a cronjob:
//
//  1. It does not support running a job at a specific time of day, but only supports the "run every X" idiom.
//  2. It is not guaranteed to run every time the schedule is hit. The next run of the TaskFun will occur only
//     AFTER the current execution is finished
//  3. As a side effect of #2, this guarantees that only one TaskFun is executing at any one time.
//
// Under the hood, [<- time.After(dur)] is used to set the schedule, so it is susceptible to time traveling
// if the system clock changes.
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

func buildTask[S any, A any](self erl.PID, taskFun func(self erl.PID, state S) (S, error), initFun func(self erl.PID, args A) (S, error), args A, opts ...StartOpt) (rtConfig[S, A], *taskOpts) {
	topts := defaultTaskOpts()
	for _, opt := range opts {
		topts = opt(topts)
	}
	return rtConfig[S, A]{taskFun: taskFun, initFun: initFun, taskArgs: args, loopTimeout: topts.loopTimeout}, topts
}

// See [StartLink]
func Start[S any, A any](self erl.PID, taskFun func(self erl.PID, state S) (S, error), initFun func(self erl.PID, args A) (S, error), args A, opts ...StartOpt) (erl.PID, error) {
	c, topts := buildTask[S, A](self, taskFun, initFun, args, opts...)
	return genserver.Start[rtState[S, A]](self, &rtSrv[S, A]{}, c, genserver.InheritOpts(topts))
}

// The [initFun] will run once, when the process starts, then the [taskFun] will
// run on every `Interval` or every 5m if [SetInterval] is not used
func StartLink[S any, A any](self erl.PID, taskFun func(self erl.PID, state S) (S, error), initFun func(self erl.PID, args A) (S, error), args A, opts ...StartOpt) (erl.PID, error) {
	c, topts := buildTask[S, A](self, taskFun, initFun, args, opts...)
	return genserver.StartLink[rtState[S, A]](self, &rtSrv[S, A]{}, c, genserver.InheritOpts(topts))
}

// See [StartLink]
func StartMonitor[S any, A any](self erl.PID, taskFun func(self erl.PID, state S) (S, error), initFun func(self erl.PID, args A) (S, error), args A, opts ...StartOpt) (erl.PID, erl.Ref, error) {
	c, topts := buildTask[S, A](self, taskFun, initFun, args, opts...)
	return genserver.StartMonitor[rtState[S, A]](self, &rtSrv[S, A]{}, c, genserver.InheritOpts(topts))
}

// Stops the task fun, synchronously. A running TaskFun will complete, possibly
// after this function returns.
func Stop(self erl.PID, task erl.PID, opts ...genserver.ExitOpt) error {
	return genserver.Stop(self, task, opts...)
}
