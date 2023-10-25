package application

import (
	"context"
	"fmt"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
)

type Application interface {
	Start(self erl.PID, args any) (erl.PID, error)
	Stop() error
}

// Starts a process with the Application callback. This is the root
// of an app, and can be used to start supervisors of worker processes.
func Start(app Application, args any, cancel context.CancelFunc) *App {
	wait := make(chan error)
	ap := &App{app: app, notify: wait, cancel: cancel}
	selfPID := erl.Spawn(ap)
	ap.self = selfPID

	return ap
}

type App struct {
	app     Application
	args    any
	rootSup erl.PID
	notify  chan<- error
	cancel  context.CancelFunc
	stopped bool
	self    erl.PID
}

// stops the app, sending [exitreason.SupervisorShutdown] to itself to all its children, calls the Application.Stop() method on the app.
func (ap *App) Stop() error {
	// TODO: log something about the app being asked to stop
	ap.stopped = true
	stop := ap.app.Stop()
	erl.Exit(erl.RootPID(), ap.self, exitreason.SupervisorShutdown)
	return stop
}

// Get the applications pid.
//
// WARNING: linking against this pid will mean your whole application shuts down if the
// linked process crashes. Most apps will only link supervisors to this process.
func (ap *App) Self() erl.PID {
	return ap.self
}

func (ap *App) Stopped() bool {
	return ap.stopped
}

func (ap *App) Receive(self erl.PID, inbox <-chan any) error {
	supPID, startErr := ap.app.Start(self, ap.args)
	erl.ProcessFlag(self, erl.TrapExit, true)
	if startErr != nil {
		ap.notify <- startErr
		return startErr
	}
	for x := range inbox {
		switch msg := x.(type) {
		case erl.ExitMsg:

			if msg.Proc == supPID {
				erl.Logger.Println("App got exitmsg from the root supervisor")
				ap.cancel()
				// ap.notify <- msg.Reason
				return exitreason.Normal
			}
		default:
			erl.DebugPrintf("Application got message that wasn't an exit: %+v", msg)
		}
	}
	ap.cancel()
	return exitreason.Exception(fmt.Errorf("Application exited because the inbox was closed"))
}
