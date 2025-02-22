package testserver

import (
	"errors"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
	"github.com/uberbrodt/erl-go/erl/gensrv"
	"github.com/uberbrodt/erl-go/erl/supervisor"
)

// TestServer

// The initial Arg that is received in the Init callback/initially set in StartLink
type TestServerConfig struct {
	// the function that will be used as the init handler
	InitFn func(self erl.PID, args any) (TestServer, any, error)
}

type TestServer struct {
	Conf TestServerConfig
}

// Adds a named process to a Supervisor. Many servers are autonomous and do not need a Name, so consider refactoring if that's the case
func ChildSpec(id string, config TestServerConfig, opts ...supervisor.ChildSpecOpt) supervisor.ChildSpec {
	return supervisor.NewChildSpec(id,
		func(self erl.PID) (erl.PID, error) {
			return StartLink(self, config)
		}, opts...,
	)
}

// Start and link the [TestServer], using handlers based on values set in the [TestServerConfig]
func StartLink(self erl.PID, conf TestServerConfig) (erl.PID, error) {
	return gensrv.StartLink[TestServer](self, conf,
		gensrv.RegisterInit(conf.InitFn),
	)
}

// Start and monitor the [TestServer], using handlers based on values set in the [TestServerConfig]
func StartMonitor(self erl.PID, conf TestServerConfig) (erl.PID, erl.Ref, error) {
	return gensrv.StartMonitor(self, conf,
		gensrv.RegisterInit(conf.InitFn),
	)
}

// Start [TestServer], using handlers based on values set in the [TestServerConfig]. This
// process is unlinked
func Start(self erl.PID, conf TestServerConfig) (erl.PID, error) {
	return gensrv.Start(self, conf,
		gensrv.RegisterInit(conf.InitFn),
	)
}

func InitOK(self erl.PID, args any) (TestServer, any, error) {
	conf, ok := args.(TestServerConfig)

	if !ok {
		return TestServer{}, nil, exitreason.Exception(errors.New("Init arg must be a {}"))
	}
	return TestServer{Conf: conf}, nil, nil
}

func InitError(self erl.PID, args any) (TestServer, any, error) {
	conf, ok := args.(TestServerConfig)

	if !ok {
		return TestServer{}, nil, exitreason.Exception(errors.New("Init arg must be a {}"))
	}
	return TestServer{Conf: conf}, nil, exitreason.Shutdown("exited in init")
}

func InitIgnore(self erl.PID, args any) (TestServer, any, error) {
	conf, ok := args.(TestServerConfig)

	if !ok {
		return TestServer{}, nil, exitreason.Exception(errors.New("Init arg must be a {}"))
	}
	return TestServer{Conf: conf}, nil, exitreason.Ignore
}
