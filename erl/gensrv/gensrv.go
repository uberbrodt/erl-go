/*
Package gensrv provides a registration-based GenServer implementation.

This is a higher-level alternative to the genserver package that allows you to
register handlers for specific message types using functional options, rather
than implementing the full GenServer interface.

# Basic Usage

	pid, err := gensrv.StartLink[MyState](self, nil,
		gensrv.RegisterInit(func(self erl.PID, arg any) (MyState, any, error) {
			return MyState{Count: 0}, nil, nil
		}),
		gensrv.RegisterCall(GetCount{}, func(self erl.PID, req GetCount, from genserver.From, state MyState) (genserver.CallResult[MyState], error) {
			return genserver.CallResult[MyState]{Msg: state.Count, State: state}, nil
		}),
	)

# Panic Recovery

All panics in registered handlers are automatically caught at the Process level.
You do not need to add defer/recover in your handler implementations. When a
panic occurs in any registered callback (Init, Call, Cast, Info, Continue,
Terminate), the process exits cleanly and supervision trees can restart it
from fresh state.

See the genserver package documentation for more details on panic recovery behavior.
*/
package gensrv

import (
	"reflect"
	"time"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/genserver"
)

func (o *config[State]) SetName(name erl.Name) {
	o.name = name
}

func (o *config[State]) GetName() erl.Name {
	return o.name
}

func (o *config[State]) SetStartTimeout(tout time.Duration) {
	o.startTimeout = tout
}

func (o *config[State]) GetStartTimeout() time.Duration {
	return o.startTimeout
}

// CastHandle represents an asynchronous message handler.
// Arg is the message type to match against in the handler registry.
// Handler processes the message and returns the updated state, an optional continuation term, and any error.
type CastHandle[State any] struct {
	Arg     any
	Handler func(self erl.PID, request any, state State) (newState State, continueTerm any, err error)
}

// CallHandle represents a synchronous request handler.
// Arg is the message type to match against in the handler registry.
// Handler processes the request and returns a result to the caller along with the updated state.
type CallHandle[State any] struct {
	Arg     any
	Handler func(self erl.PID, request any, from genserver.From, state State) (result genserver.CallResult[State], err error)
}

// GenSrvOpt is a function that configures a GenServer instance.
// It modifies the internal config of the server to customize its behavior.
type GenSrvOpt[State any] func(c *config[State])

// Start creates and initializes a new GenServer process.
// The 'self' parameter is the PID of the calling process.
// The 'arg' parameter is passed to the initialization function.
// The optional opts parameters allow customizing the server's behavior.
// Returns the PID of the new process or an error if initialization fails.
func Start[State any](self erl.PID, arg any, opts ...GenSrvOpt[State]) (erl.PID, error) {
	conf := doConf[State](opts...)

	return genserver.Start[State](self, &CB[State]{conf: conf}, arg, genserver.InheritOpts(conf))
}

// StartLink creates and initializes a new GenServer process linked to the calling process.
// If either process terminates, the other receives an exit signal.
// The 'self' parameter is the PID of the calling process.
// The 'arg' parameter is passed to the initialization function.
// The optional opts parameters allow customizing the server's behavior.
func StartLink[State any](self erl.PID, arg any, opts ...GenSrvOpt[State]) (erl.PID, error) {
	conf := doConf[State](opts...)
	return genserver.StartLink[State](self, &CB[State]{conf: conf}, arg, genserver.InheritOpts(conf))
}

// StartMonitor creates and initializes a new GenServer process while monitoring it.
// The 'self' parameter is the PID of the calling process that will receive monitor notifications.
// The 'arg' parameter is passed to the initialization function.
// The optional opts parameters allow customizing the server's behavior.
// Returns the PID, a monitor reference, and any initialization error.
func StartMonitor[State any](self erl.PID, arg any, opts ...GenSrvOpt[State]) (erl.PID, erl.Ref, error) {
	conf := doConf[State](opts...)
	return genserver.StartMonitor[State](self, &CB[State]{conf: conf}, arg, genserver.InheritOpts(conf))
}

func doConf[State any](opts ...GenSrvOpt[State]) *config[State] {
	conf := &config[State]{
		castFuns:     make(map[reflect.Type]func(self erl.PID, arg any, state State) (newState State, continu any, err error)),
		callFuns:     make(map[reflect.Type]func(self erl.PID, request any, from genserver.From, state State) (genserver.CallResult[State], error)),
		infoFuns:     make(map[reflect.Type]func(self erl.PID, arg any, state State) (newState State, continu any, err error)),
		continueFuns: make(map[reflect.Type]func(self erl.PID, contTerm any, state State) (newState State, continu any, err error)),
	}

	for _, opt := range opts {
		opt(conf)
	}
	return conf
}

// SetName provides a name for the GenServer process, allowing it to be referenced by name
// instead of only by PID. Named processes can be found using erl.WhereIs(name).
func SetName[State any](name erl.Name) GenSrvOpt[State] {
	return func(c *config[State]) {
		c.name = name
	}
}

// SetStartTimeout configures the maximum time duration allowed for the initialization phase.
// If initialization takes longer than this duration, the process will be terminated.
func SetStartTimeout[State any](tout time.Duration) GenSrvOpt[State] {
	return func(c *config[State]) {
		c.startTimeout = tout
	}
}

// RegisterInit registers an initialization function that's called when the server starts.
// The function should return the initial state, an optional continuation term, and any error.
// If an error is returned, the server will terminate.
// If exitreason.Ignore is returned as an error, the server will shut down normally without an error.
func RegisterInit[State any, Arg any](init func(self erl.PID, arg Arg) (State, any, error)) GenSrvOpt[State] {
	return func(c *config[State]) {
		c.initFun = func(self erl.PID, a any) (genserver.InitResult[State], error) {
			msg := a.(Arg)
			s, c, err := init(self, msg)
			return genserver.InitResult[State]{Continue: c, State: s}, err
		}
	}
}

// RegisterCast registers a handler for asynchronous messages of a specific type.
// The matchType parameter is a zero value of the message type to match (e.g., MyMessage{}).
// The handler function receives the server PID, the cast message, and the current state.
// It returns the updated state, an optional continuation term, and any error.
// Cast messages don't expect a response from the server.
func RegisterCast[State any, Msg any](matchType any, fn func(self erl.PID, a Msg, state State) (newState State, continu any, err error)) GenSrvOpt[State] {
	return func(c *config[State]) {
		termT := reflect.TypeOf(matchType)
		c.castFuns[termT] = func(self erl.PID, m any, s State) (newState State, continu any, err error) {
			msg := m.(Msg)
			return fn(self, msg, s)
		}
	}
}

// RegisterInfo registers a handler for information messages of a specific type.
// Info messages are typically system messages or messages from other processes
// that aren't explicitly part of the server's API.
// The matchType parameter is a zero value of the message type to match.
// The handler function behaves similarly to cast handlers but is used for different purposes.
func RegisterInfo[State any, Msg any](matchType any, fn func(self erl.PID, a Msg, state State) (newState State, continu any, err error)) GenSrvOpt[State] {
	return func(c *config[State]) {
		termT := reflect.TypeOf(matchType)
		c.infoFuns[termT] = func(self erl.PID, m any, s State) (newState State, continu any, err error) {
			msg := m.(Msg)
			return fn(self, msg, s)
		}
	}
}

// RegisterCall registers a handler for synchronous request messages of a specific type.
// The matchType parameter is a zero value of the message type to match.
// The handler function receives the server PID, the request message,
// the caller's From reference, and the current state.
// It returns a CallResult containing the response and updated state, along with any error.
// Call messages expect a response which is sent back to the caller.
func RegisterCall[State any, Msg any](matchType any, fn func(self erl.PID, request Msg, from genserver.From, state State) (result genserver.CallResult[State], err error)) GenSrvOpt[State] {
	return func(c *config[State]) {
		termT := reflect.TypeOf(matchType)
		c.callFuns[termT] = func(self erl.PID, m any, f genserver.From, s State) (result genserver.CallResult[State], err error) {
			msg := m.(Msg)
			return fn(self, msg, f, s)
		}
	}
}

// RegisterContinue registers a handler for continuation messages generated by other handlers.
// Continuations allow for deferred or scheduled processing after a call, cast, or info message.
// The matchType parameter is a zero value of the message type to match.
// Continuation handlers can chain by returning another continuation term.
func RegisterContinue[State any, Msg any](matchType any, fn func(self erl.PID, cont Msg, state State) (newState State, continu any, err error)) GenSrvOpt[State] {
	return func(c *config[State]) {
		termT := reflect.TypeOf(matchType)
		c.continueFuns[termT] = func(self erl.PID, m any, s State) (newState State, continu any, err error) {
			msg := m.(Msg)
			return fn(self, msg, s)
		}
	}
}

// RegisterTerminate registers a handler that's called when the server is about to terminate.
// It can perform cleanup operations but cannot prevent termination.
// The reason parameter contains the error that caused termination.
func RegisterTerminate[State any](terminate func(self erl.PID, reason error, state State)) GenSrvOpt[State] {
	return func(c *config[State]) {
		c.terminateFun = terminate
	}
}
