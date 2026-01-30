/*
An application is a sort of root for a supervision tree. It's primary function is
to call a [context.CancelFunc] when the children crashes exceed the restart intensity.

While it does wrap a [supervisor], it is intentially not configurable. The idea is that
that's a domain specific problem that should be handled in your app.

For most apps, something like this should be in your [main]:

	package main

	import (
		"context"

		"github.com/uberbrodt/erl-go/erl"
		"github.com/uberbrodt/erl-go/erl/application"
		"github.com/uberbrodt/erl-go/erl/supervisor"

	)

	var App *application.App

	func init() {
		// set to true to get a lot of logs
		erl.SetDebugLog(false)
	}

	type MyApp struct{}

	 // Pass in config via the args parameter
	func (ba *MyApp) Start(self erl.PID, args any) (erl.PID, error) {

	 // start children based on config
		children := []supervisor.ChildSpec{ }
		return supervisor.StartDefaultLink(self, children, supervisor.NewSupFlags())
	}

	// do any cleanup here. should run unless there's a fatal error in the runtime.
	func (ba *MyApp) Stop() error {
		return nil
	}

	func main() {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()

		 // parse flags, load a real config here.
		 conf := make(map[string]any)
		 App := application.Start(&MyApp{}, conf, cancel)

		<-ctx.Done()
		if !app.App.Stopped() {
			app.App.Stop()
		}

	}
*/
package application

import (
	"context"

	"github.com/uberbrodt/erl-go/chronos"
	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
	"github.com/uberbrodt/erl-go/erl/genserver"
	"github.com/uberbrodt/erl-go/erl/gensrv"
)

type Application interface {
	Start(self erl.PID, args any) (erl.PID, error)
	Stop() error
}

// appState holds the state for the application GenServer.
type appState struct {
	app     Application
	rootSup erl.PID
	cancel  context.CancelFunc
	stopped bool
}

// appArgs holds the arguments passed to the application init.
type appArgs struct {
	app    Application
	args   any
	cancel context.CancelFunc
}

// Starts a process with the Application callback. This is the root
// of an app, and can be used to start supervisors of worker processes.
func Start(app Application, args any, cancel context.CancelFunc) *App {
	initArgs := appArgs{
		app:    app,
		args:   args,
		cancel: cancel,
	}

	selfPID, err := gensrv.StartLink(erl.RootPID(), initArgs, buildOpts()...)
	if err != nil {
		panic(err)
	}

	return &App{
		app:  app,
		self: selfPID,
	}
}

// App is the handle returned by [Start] that allows controlling the application.
type App struct {
	app     Application
	self    erl.PID
	stopped bool
}

// Stop stops the app synchronously, shutting down the supervision tree and calling
// the Application.Stop() method.
func (ap *App) Stop() error {
	erl.Logger.Println("Application asked to stop")
	ap.stopped = true
	stopErr := ap.app.Stop()
	erl.Logger.Println("Application shutting down supervision tree...")

	// Use genserver.Stop for synchronous shutdown.
	// This will trigger handleTerminate which shuts down the supervisor.
	err := genserver.Stop(erl.RootPID(), ap.self,
		genserver.StopReason(exitreason.SupervisorShutdown),
		genserver.StopTimeout(chronos.Dur("60s")))
	if err != nil {
		erl.Logger.Printf("Application stop error: %v", err)
	}

	erl.Logger.Println("Done")
	return stopErr
}

// Stopped returns true if the application has been stopped.
func (ap *App) Stopped() bool {
	return ap.stopped || !erl.IsAlive(ap.self)
}

func buildOpts() []gensrv.GenSrvOpt[appState] {
	return []gensrv.GenSrvOpt[appState]{
		gensrv.RegisterInit(appInit),
		gensrv.RegisterInfo(erl.ExitMsg{}, handleExitMsg),
		gensrv.RegisterTerminate(handleTerminate),
	}
}

func appInit(self erl.PID, args any) (appState, any, error) {
	initArgs := args.(appArgs)

	// Start the application's supervision tree
	supPID, startErr := initArgs.app.Start(self, initArgs.args)
	if startErr != nil {
		return appState{}, nil, startErr
	}

	// Trap exits so we receive ExitMsg from children
	erl.ProcessFlag(self, erl.TrapExit, true)

	state := appState{
		app:     initArgs.app,
		rootSup: supPID,
		cancel:  initArgs.cancel,
		stopped: false,
	}

	return state, nil, nil
}

// handleExitMsg handles exit messages from linked processes.
// This is called when the root supervisor exits (e.g., restart intensity exceeded).
func handleExitMsg(self erl.PID, msg erl.ExitMsg, state appState) (appState, any, error) {
	if msg.Proc == state.rootSup {
		// Root supervisor exited - this means restart intensity was exceeded
		erl.Logger.Println("App got exitmsg from the root supervisor")
		state.stopped = true
		state.cancel()
		return state, nil, exitreason.Normal
	}

	// Other exit messages - just log and continue
	erl.DebugPrintf("Application got ExitMsg from %v: %v", msg.Proc, msg.Reason)
	return state, nil, nil
}

// handleTerminate is called when the application GenServer is terminating.
// This happens when genserver.Stop() is called (from App.Stop()) or on any other termination
// (because we are trapping exits).
func handleTerminate(self erl.PID, reason error, state appState) {
	erl.Logger.Printf("Application terminating with reason: %v", reason)

	// Shut down the supervisor if it's still alive
	if erl.IsAlive(state.rootSup) {
		erl.Logger.Println("App: shutting down supervisor")
		genserver.Stop(self, state.rootSup, genserver.StopReason(exitreason.SupervisorShutdown)) //nolint:errcheck
		erl.Logger.Println("App: supervisor stopped")
	}

	// Ensure cancel is called on any termination
	if !state.stopped {
		state.cancel()
	}
}
