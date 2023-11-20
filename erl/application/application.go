package application

import (
	"context"
	"fmt"

	"github.com/uberbrodt/erl-go/chronos"
	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
	"github.com/uberbrodt/erl-go/erl/genserver"
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
	erl.Logger.Println("Application asked to stop")
	ap.stopped = true
	stop := ap.app.Stop()
	erl.Logger.Println("Application shutting down supervision tree...")
	// NOTE: the application process isn't a GenServer (currently) but [genserver.Stop] will provide a synchronous stop
	genserver.Stop(erl.RootPID(), ap.self, genserver.StopReason(exitreason.SupervisorShutdown), genserver.StopTimeout(chronos.Dur("60s")))
	erl.Logger.Println("Done")
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
				return exitreason.Normal
			} else if msg.Proc == erl.RootPID() {
				genserver.Stop(self, supPID, genserver.StopReason(exitreason.SupervisorShutdown))
			}
		default:
			erl.DebugPrintf("Application got message that wasn't an exit: %+v", msg)
		}
	}
	ap.cancel()
	return exitreason.Exception(fmt.Errorf("Application exited because the inbox was closed"))
}
