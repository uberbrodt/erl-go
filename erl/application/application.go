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
	ap := &App{app: app, args: args, notify: wait, cancel: cancel}
	selfPID := erl.Spawn(ap)
	ap.self = selfPID

	result := <-wait
	if result != nil {
		panic(result)
	}

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
	erl.Logger.Println("Application asked to stop")
	ap.stopped = true
	stop := ap.app.Stop()
	erl.Logger.Println("Application shutting down supervision tree...")
	// NOTE: the application process isn't a GenServer (currently) but [genserver.Stop] will provide a synchronous stop
	genserver.Stop(ap.self, ap.self, genserver.StopReason(exitreason.SupervisorShutdown), genserver.StopTimeout(chronos.Dur("60s")))
	erl.Logger.Println("Done")
	return stop
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
	close(ap.notify)

	for x := range inbox {
		switch msg := x.(type) {
		case erl.ExitMsg:
			if msg.Proc == supPID {
				erl.Logger.Println("App got exitmsg from the root supervisor")
				ap.stopped = true
				ap.cancel()
				return exitreason.Normal
			} else if !msg.Link {
				erl.Logger.Println("App: shutting down supervisor")
				genserver.Stop(self, supPID, genserver.StopReason(exitreason.SupervisorShutdown))
				erl.Logger.Println("App: supervisor done, calling CancelFunc")
				ap.cancel()
				erl.Logger.Println("App: cancel finished")
				return exitreason.SupervisorShutdown
			}
		default:
			erl.DebugPrintf("Application got message that wasn't an exit: %+v", msg)
		}
	}
	// NOTE: the following lines shouldn't be reachable in production code.
	ap.cancel()
	return exitreason.Exception(fmt.Errorf("Application exited because the inbox was closed"))
}
