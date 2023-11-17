package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/uberbrodt/erl-go/chronos"
	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/application"
	"github.com/uberbrodt/erl-go/erl/supervisor"
	"github.com/uberbrodt/erl-go/erl/task"
)

var App *application.App

var httpSrv *http.Server

var httpSrvName erl.Name = erl.Name("http_srv")

var httpSrvStart supervisor.StartFunSpec = func(self erl.PID) (erl.PID, error) {
	return task.StartLink(self, func() error {
		log.Printf("HTTPSrv Starting...")
		httpSrv = &http.Server{Addr: ":4040"}
		// return errors.New(("uh-oh"))
		return httpSrv.ListenAndServe()
	}, func() error {
		log.Printf("HTTPSrv shutting down...")
		return httpSrv.Shutdown(context.Background())
	}, task.SetName(httpSrvName))
}

type BasicApp struct{}

func (ba *BasicApp) Start(self erl.PID, args any) (erl.PID, error) {
	children := []supervisor.ChildSpec{
		supervisor.NewChildSpec("http_srv", httpSrvStart, supervisor.SetRestart(supervisor.Permanent)),
	}
	return supervisor.StartDefaultLink(self, children, supervisor.NewSupFlags())
}

func (ba *BasicApp) Stop() error {
	return nil
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	app := &BasicApp{}

	App = application.Start(app, nil, cancel)
	log.Printf("warming up...")
	<-time.After(chronos.Dur("3s"))

	taskPID, _ := erl.WhereIs(httpSrvName)
	task.Stop(taskPID)

	<-ctx.Done()
	log.Println("context is finished")
	if !App.Stopped() {
		App.Stop()
	}
}
