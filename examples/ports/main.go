package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/application"
	"github.com/uberbrodt/erl-go/erl/genserver"
	"github.com/uberbrodt/erl-go/erl/supervisor"
)

var App *application.App

type BasicApp struct{}

func (ba *BasicApp) Start(self erl.PID, args any) (erl.PID, error) {
	children := []supervisor.ChildSpec{
		supervisor.NewChildSpec("bean_counter", func(self erl.PID) (erl.PID, error) {
			return genserver.StartLink[MyServerState](self, MyServer{}, nil, genserver.SetName("bean_counter"))
		}, supervisor.SetRestart(supervisor.Permanent)),
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

	<-ctx.Done()
	if !App.Stopped() {
		App.Stop()
	}
}
