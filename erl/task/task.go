// WARNING: this package is EXPERIMENTAL and may change in breaking ways
//
// tasks are basically wrappers around go routines that will send exits if the specified
// action is given.
//
// The process traps exits and will call the [cleanup] function before it exits. this makes it
// a nice fit for things like [net/http.Server]s that generally block a thread but also can be closed
// using a pointer.
package task

import (
	"fmt"
	"time"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
)

type taskOpts struct {
	name         erl.Name
	startTimeout time.Duration
}

type StartOpt func(opts taskOpts) taskOpts

func SetName(name erl.Name) StartOpt {
	return func(opts taskOpts) taskOpts {
		opts.name = name
		return opts
	}
}

func Start(self erl.PID, taskFun func() error, cleanupFun func() error, opts ...StartOpt) (erl.PID, error) {
	topts := taskOpts{}
	for _, opt := range opts {
		topts = opt(topts)
	}

	t := &Task{taskFun: taskFun, cleanup: cleanupFun, parent: self, opts: topts}

	pid := erl.Spawn(t)
	return pid, nil
}

func StartLink(self erl.PID, taskFun func() error, cleanupFun func() error, opts ...StartOpt) (erl.PID, error) {
	topts := taskOpts{}
	for _, opt := range opts {
		topts = opt(topts)
	}

	t := &Task{taskFun: taskFun, cleanup: cleanupFun, parent: self, opts: topts}

	pid := erl.SpawnLink(self, t)
	return pid, nil
}

func StartMonitor(self erl.PID, taskFun func() error, cleanupFun func() error, opts ...StartOpt) (erl.PID, erl.Ref, error) {
	topts := taskOpts{}
	for _, opt := range opts {
		topts = opt(topts)
	}
	t := &Task{taskFun: taskFun, cleanup: cleanupFun, parent: self, opts: topts}

	pid, ref := erl.SpawnMonitor(self, t)
	return pid, ref, nil
}

func Stop(task erl.PID) error {
	// TODO: this should be synchronous. Because it's currently not, error is always nil
	erl.Send(task, stopTask{})
	return nil
}

type Task struct {
	taskFun func() error
	cleanup func() error
	parent  erl.PID
	opts    taskOpts
}

type taskFunExited struct {
	err error
}

type stopTask struct{}

func (t *Task) Receive(self erl.PID, inbox <-chan any) error {
	if t.opts.name != "" {
		if err := erl.Register(t.opts.name, self); err != nil {
			return err
		}
	}
	erl.ProcessFlag(self, erl.TrapExit, true)
	go func() {
		err := t.taskFun()
		erl.Send(self, taskFunExited{err: err})
	}()

	for {
		anymsg, ok := <-inbox

		if !ok {
			return nil
		}

		switch msg := anymsg.(type) {
		case erl.ExitMsg:
			if msg.Proc.Equals(t.parent) {
				err := t.cleanup()
				return exitreason.Shutdown(fmt.Errorf("task exited because the parent process exited: %w", err))
			}
		case taskFunExited:
			if msg.err == nil {
				return exitreason.Normal
			} else {
				return exitreason.Exception(msg.err)
			}
		case stopTask:
			err := t.cleanup()
			if err == nil {
				return exitreason.Normal
			}
			return exitreason.Exception(err)
		}
	}
}
