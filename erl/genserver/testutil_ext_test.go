package genserver_test

import (
	"errors"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
	"github.com/uberbrodt/erl-go/erl/gensrv"
	"github.com/uberbrodt/erl-go/erl/supervisor"
)

type DownNotification struct {
	msg erl.DownMsg
}

// The initial Arg that is received in the Init callback/initially set in StartLink
type TestParentConfig struct {
	// the process that will be relayed down messages
	Relay erl.PID
}

type TestParent struct {
	conf TestParentConfig
}

// Adds a named process to a Supervisor. Many servers are autonomous and do not need a Name, so consider refactoring if that's the case
func ParentTestChildSpec(id string, config TestParentConfig, opts ...supervisor.ChildSpecOpt) supervisor.ChildSpec {
	return supervisor.NewChildSpec(id,
		func(self erl.PID) (erl.PID, error) {
			return ParentTestStartLink(self, config)
		}, opts...,
	)
}

// Returns a worker/normal process that will forward down messages to a configure process
func ParentTestStartLink(self erl.PID, conf TestParentConfig) (erl.PID, error) {
	return gensrv.StartLink[TestParent](self, conf,
		gensrv.RegisterInit(parentInit),
		gensrv.RegisterInfo(erl.DownMsg{}, parentGetsDownMsg),
	)
}

// Initialization function. Called when a process is started. Supervisors will block until this function returns.
func parentInit(self erl.PID, args any) (TestParent, any, error) {
	conf, ok := args.(TestParentConfig)

	if !ok {
		return TestParent{}, nil, exitreason.Exception(errors.New("Init arg must be a {}"))
	}
	return TestParent{conf: conf}, nil, nil
}

func parentGetsDownMsg(self erl.PID, msg erl.DownMsg, state TestParent) (TestParent, any, error) {
	erl.Send(state.conf.Relay, DownNotification{msg: msg})
	return state, nil, nil
}

// TestServer

// The initial Arg that is received in the Init callback/initially set in StartLink
type TestServerConfig struct {
	// the process that will be relayed down messages
	Relay  erl.PID
	InitFn func(self erl.PID, args any) (TestServer, any, error)
}

type TestServer struct {
	conf TestServerConfig
}

// Adds a named process to a Supervisor. Many servers are autonomous and do not need a Name, so consider refactoring if that's the case
func ServerTestChildSpec(id string, config TestServerConfig, opts ...supervisor.ChildSpecOpt) supervisor.ChildSpec {
	return supervisor.NewChildSpec(id,
		func(self erl.PID) (erl.PID, error) {
			return ServerTestStartLink(self, config)
		}, opts...,
	)
}

// Start and link the [TestServer], using handlers based on values set in the [TestServerConfig]
func ServerTestStartLink(self erl.PID, conf TestServerConfig) (erl.PID, error) {
	return gensrv.StartLink[TestServer](self, conf,
		gensrv.RegisterInit(conf.InitFn),
	)
}

// Start and monitor the [TestServer], using handlers based on values set in the [TestServerConfig]
func ServerTestStartMonitor(self erl.PID, conf TestServerConfig) (erl.PID, erl.Ref, error) {
	return gensrv.StartMonitor(self, conf,
		gensrv.RegisterInit(conf.InitFn),
	)
}

// Start [TestServer], using handlers based on values set in the [TestServerConfig]. This
// process is unlinked
func ServerTestStart(self erl.PID, conf TestServerConfig) (erl.PID, error) {
	return gensrv.Start(self, conf,
		gensrv.RegisterInit(conf.InitFn),
	)
}

func ServerTestInitOK(self erl.PID, args any) (TestServer, any, error) {
	conf, ok := args.(TestServerConfig)

	if !ok {
		return TestServer{}, nil, exitreason.Exception(errors.New("Init arg must be a {}"))
	}
	return TestServer{conf: conf}, nil, nil
}

func ServerTestInitError(self erl.PID, args any) (TestServer, any, error) {
	conf, ok := args.(TestServerConfig)

	if !ok {
		return TestServer{}, nil, exitreason.Exception(errors.New("Init arg must be a {}"))
	}
	return TestServer{conf: conf}, nil, exitreason.Shutdown("exited in init")
}

func ServerTestInitIgnore(self erl.PID, args any) (TestServer, any, error) {
	conf, ok := args.(TestServerConfig)

	if !ok {
		return TestServer{}, nil, exitreason.Exception(errors.New("Init arg must be a {}"))
	}
	return TestServer{conf: conf}, nil, exitreason.Ignore
}
