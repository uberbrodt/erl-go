/*
testserver provides a [GenSrv] that can be configured with different msg handlers as needed.
Useful for building a fake/stub implementation or for testing GenServer behaviour inside of the
erl-go library.

# Example Usage
*/
package testserver

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
	"github.com/uberbrodt/erl-go/erl/genserver"
	"github.com/uberbrodt/erl-go/erl/gensrv"
	"github.com/uberbrodt/erl-go/erl/supervisor"
)

// Create a new [Config]. Sets default [InitFn] to [InitOK].
func NewConfig() *Config {
	return &Config{
		InitFn:  InitOK,
		castFns: make(map[any]func(self erl.PID, arg any, state TestServer) (newState TestServer, continu any, err error)),
		infoFns: make(map[any]func(self erl.PID, arg any, state TestServer) (newState TestServer, continu any, err error)),
		callFns: make(map[any]func(self erl.PID, request any, from genserver.From, state TestServer) (genserver.CallResult[TestServer], error)),
	}
}

// The initial Arg that is received in the Init callback/initially set in StartLink
type Config struct {
	// the function that will be used as the init handler
	InitFn      func(self erl.PID, args any) (TestServer, any, error)
	TerminateFn func(self erl.PID, reason error, state TestServer)
	castFns     map[any]func(self erl.PID, arg any, state TestServer) (newState TestServer, continu any, err error)
	infoFns     map[any]func(self erl.PID, arg any, state TestServer) (newState TestServer, continu any, err error)
	callFns     map[any]func(self erl.PID, request any, from genserver.From, state TestServer) (genserver.CallResult[TestServer], error)
}

// Set the Init Function for the GenServer. This will be called on start and every restart of a process
func (c *Config) SetInit(InitFn func(self erl.PID, args any) (TestServer, any, error)) *Config {
	c.InitFn = InitFn
	return c
}

// SetTerminate sets the Terminate handler for the GenServer. This will be called when the process terminates.
func (c *Config) SetTerminate(fn func(self erl.PID, reason error, state TestServer)) *Config {
	c.TerminateFn = fn
	return c
}

// Add a Cast handler for the TestServer. If there is already a handler specified for [msg] then this call will panic.
func (c *Config) AddCastHandler(msg any, fn func(self erl.PID, arg any, state TestServer) (newState TestServer, continu any, err error)) *Config {
	if _, ok := c.castFns[msg]; ok {
		panic(fmt.Errorf("a handler for %T already exists", msg))
	}

	c.castFns[msg] = fn
	return c
}

// Add a Info handler function for [msg]. If there is already a handler added for [msg], this function will panic.
func (c *Config) AddInfoHandler(msg any, fn func(self erl.PID, arg any, state TestServer) (newState TestServer, continu any, err error)) *Config {
	if _, ok := c.infoFns[msg]; ok {
		panic(fmt.Errorf("a handler for %T already exists", msg))
	}
	fmt.Printf("TestServer added a handler for %T", msg)

	c.infoFns[msg] = fn
	return c
}

// Add a Call handler function for [msg]. If there is already a handler added for [msg], this function will panic.
func (c *Config) AddCallHandler(msg any, fn func(self erl.PID, request any, from genserver.From, state TestServer) (genserver.CallResult[TestServer], error)) *Config {
	t := reflect.TypeOf(msg)

	if _, ok := c.callFns[t]; ok {
		panic(fmt.Errorf("a handler for %T already exists", msg))
	}

	c.callFns[t] = fn
	return c
}

type TestServer struct {
	Conf *Config
}

// Adds a named process to a Supervisor. Many servers are autonomous and do not need a Name, so consider refactoring if that's the case
func ChildSpec(id string, config *Config, opts ...supervisor.ChildSpecOpt) supervisor.ChildSpec {
	return supervisor.NewChildSpec(id,
		func(self erl.PID) (erl.PID, error) {
			return StartLink(self, config)
		}, opts...,
	)
}

func buildOpts(conf *Config) []gensrv.GenSrvOpt[TestServer] {
	opts := []gensrv.GenSrvOpt[TestServer]{}

	opts = append(opts, gensrv.RegisterInit(conf.InitFn))
	for msg, fn := range conf.castFns {
		opts = append(opts, gensrv.RegisterCast(msg, fn))
	}

	for msg, fn := range conf.infoFns {
		opts = append(opts, gensrv.RegisterInfo(msg, fn))
	}
	for msg, fn := range conf.callFns {
		opts = append(opts, gensrv.RegisterCall(msg, fn))
	}
	if conf.TerminateFn != nil {
		opts = append(opts, gensrv.RegisterTerminate(conf.TerminateFn))
	}
	return opts
}

// Start and link the [TestServer], using handlers based on values set in the [Config]
func StartLink(self erl.PID, conf *Config) (erl.PID, error) {
	return gensrv.StartLink(self, conf, buildOpts(conf)...)
}

// Start and monitor the [TestServer], using handlers based on values set in the [Config]
func StartMonitor(self erl.PID, conf *Config) (erl.PID, erl.Ref, error) {
	return gensrv.StartMonitor(self, conf, buildOpts(conf)...)
}

// Start [TestServer], using handlers based on values set in the [Config]. This
// process is unlinked
func Start(self erl.PID, conf *Config) (erl.PID, error) {
	return gensrv.Start(self, conf, buildOpts(conf)...)
}

func InitOK(self erl.PID, args any) (TestServer, any, error) {
	conf, ok := args.(*Config)

	if !ok {
		return TestServer{}, nil, exitreason.Exception(errors.New("Init arg must be a {}"))
	}
	return TestServer{Conf: conf}, nil, nil
}

func InitError(self erl.PID, args any) (TestServer, any, error) {
	conf, ok := args.(*Config)

	if !ok {
		return TestServer{}, nil, exitreason.Exception(errors.New("Init arg must be a {}"))
	}
	return TestServer{Conf: conf}, nil, exitreason.Shutdown("exited in init")
}

func InitIgnore(self erl.PID, args any) (TestServer, any, error) {
	conf, ok := args.(*Config)

	if !ok {
		return TestServer{}, nil, exitreason.Exception(errors.New("Init arg must be a {}"))
	}
	return TestServer{Conf: conf}, nil, exitreason.Ignore
}
