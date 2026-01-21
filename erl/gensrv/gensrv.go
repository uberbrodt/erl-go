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

// CastHandle represents a registered handler for asynchronous (cast) messages.
//
// This type is used internally by [RegisterCast] to store handler functions.
// You typically don't create CastHandle directly; instead use [RegisterCast].
//
// Fields:
//   - Arg: A zero-value instance of the message type, used for type matching
//   - Handler: The function called when a matching message is received
//
// The Handler function receives the server's PID, the message, and current state.
// It returns the updated state, an optional continuation term (or nil), and any error.
// If an error is returned, the server terminates with that error as the reason.
type CastHandle[State any] struct {
	Arg     any
	Handler func(self erl.PID, request any, state State) (newState State, continueTerm any, err error)
}

// CallHandle represents a registered handler for synchronous (call) requests.
//
// This type is used internally by [RegisterCall] to store handler functions.
// You typically don't create CallHandle directly; instead use [RegisterCall].
//
// Fields:
//   - Arg: A zero-value instance of the message type, used for type matching
//   - Handler: The function called when a matching request is received
//
// The Handler function receives:
//   - self: The server's PID
//   - request: The incoming request message
//   - from: Identifies the caller, used with [genserver.Reply] for deferred responses
//   - state: The current server state
//
// It returns a [genserver.CallResult] containing the reply and updated state, plus any error.
// If an error is returned, the server terminates with that error as the reason.
type CallHandle[State any] struct {
	Arg     any
	Handler func(self erl.PID, request any, from genserver.From, state State) (result genserver.CallResult[State], err error)
}

// GenSrvOpt is a functional option for configuring a gensrv server.
//
// Options are passed to [Start], [StartLink], or [StartMonitor] to customize
// the server's behavior. Available options include:
//   - [SetName]: Register the server with a name for lookup via [erl.WhereIs]
//   - [SetStartTimeout]: Set maximum time for initialization
//   - [RegisterInit]: Set the initialization handler
//   - [RegisterCall]: Register a synchronous request handler
//   - [RegisterCast]: Register an asynchronous message handler
//   - [RegisterInfo]: Register a system/info message handler
//   - [RegisterContinue]: Register a continuation handler
//   - [RegisterTerminate]: Register a cleanup handler
type GenSrvOpt[State any] func(c *config[State])

// Start creates and initializes a new GenServer process without linking or monitoring.
//
// This is equivalent to Erlang's gen_server:start/3,4. The new process runs
// independently of the caller - neither will be notified if the other exits.
// Use this when you need a standalone GenServer not managed by a supervisor.
//
// Parameters:
//   - self: The PID of the calling process (required for Init synchronization)
//   - arg: Arguments passed to the [RegisterInit] handler
//   - opts: Functional options to configure the server (see [GenSrvOpt])
//
// Returns the PID of the new process, or an error if initialization fails.
//
// Possible errors:
//   - [exitreason.Ignore]: Init handler returned [exitreason.Ignore] (not a failure)
//   - [exitreason.Timeout]: Init took longer than the start timeout
//   - Other errors: Returned directly from the Init handler
//
// The function blocks until Init completes. For supervised processes, use
// [StartLink] instead.
func Start[State any](self erl.PID, arg any, opts ...GenSrvOpt[State]) (erl.PID, error) {
	conf := doConf[State](opts...)

	return genserver.Start[State](self, &CB[State]{conf: conf}, arg, genserver.InheritOpts(conf))
}

// StartLink creates and initializes a new GenServer process linked to the calling process.
//
// This is equivalent to Erlang's gen_server:start_link/3,4. The new process is
// atomically linked to the caller, meaning if either process exits abnormally,
// the other receives an exit signal. This is the recommended way to start
// GenServers under a supervisor.
//
// Parameters:
//   - self: The PID of the calling process (required for linking)
//   - arg: Arguments passed to the [RegisterInit] handler
//   - opts: Functional options to configure the server (see [GenSrvOpt])
//
// Returns the PID of the new process, or an error if initialization fails.
//
// Possible errors:
//   - [exitreason.Ignore]: Init handler returned [exitreason.Ignore] (not a failure)
//   - [exitreason.Timeout]: Init took longer than the start timeout
//   - Other errors: Returned directly from the Init handler
//
// The function blocks until Init completes. If Init returns an error or
// [exitreason.Ignore], the process terminates and that error is returned.
//
// Example:
//
//	pid, err := gensrv.StartLink[MyState](self, initArgs,
//		gensrv.RegisterInit[MyState, InitArgs](myInitHandler),
//		gensrv.RegisterCall[MyState, GetRequest](GetRequest{}, myCallHandler),
//	)
func StartLink[State any](self erl.PID, arg any, opts ...GenSrvOpt[State]) (erl.PID, error) {
	conf := doConf[State](opts...)
	return genserver.StartLink[State](self, &CB[State]{conf: conf}, arg, genserver.InheritOpts(conf))
}

// StartMonitor creates and initializes a new GenServer process and monitors it.
//
// This is equivalent to Erlang's gen_server:start_monitor/3,4. Unlike [StartLink],
// a monitor is one-way: if the GenServer exits, the caller receives an [erl.DownMsg]
// but is not killed. This is useful when you want to be notified of process death
// without being affected by it.
//
// Parameters:
//   - self: The PID of the calling process (receives [erl.DownMsg] on server exit)
//   - arg: Arguments passed to the [RegisterInit] handler
//   - opts: Functional options to configure the server (see [GenSrvOpt])
//
// Returns the PID, a monitor reference (for use with [erl.Demonitor]), and any error.
//
// Possible errors:
//   - [exitreason.Ignore]: Init handler returned [exitreason.Ignore] (not a failure)
//   - [exitreason.Timeout]: Init took longer than the start timeout
//   - Other errors: Returned directly from the Init handler
//
// The function blocks until Init completes. The monitor reference can be used
// to cancel monitoring via [erl.Demonitor] if the server exit notification is
// no longer needed.
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

// SetName registers the GenServer process with a name for easier lookup.
//
// Named processes can be found using [erl.WhereIs] and addressed by name
// in [genserver.Call] and [genserver.Cast] instead of requiring a PID.
// This is equivalent to registering a process name in Erlang.
//
// Parameters:
//   - name: The name to register the process under
//
// If a process with the given name already exists, the server will fail to start.
// Names are automatically unregistered when the process exits.
//
// Example:
//
//	pid, err := gensrv.StartLink[State](self, nil,
//		gensrv.SetName[State]("my_server"),
//		// ... other options
//	)
//
//	// Later, call by name instead of PID
//	result, err := genserver.Call(self, erl.Name("my_server"), GetRequest{}, timeout)
func SetName[State any](name erl.Name) GenSrvOpt[State] {
	return func(c *config[State]) {
		c.name = name
	}
}

// SetStartTimeout configures the maximum time allowed for the initialization phase.
//
// If the [RegisterInit] handler takes longer than this duration:
//   - The process is killed with [exitreason.Kill]
//   - [exitreason.Timeout] is returned from [Start], [StartLink], or [StartMonitor]
//
// The default timeout is 5 seconds. Use [timeout.Infinity] for no timeout,
// though this is not recommended as a hanging Init will block the caller indefinitely.
//
// Parameters:
//   - tout: Maximum duration to wait for Init to complete
//
// Example:
//
//	gensrv.StartLink[State](self, nil,
//		gensrv.SetStartTimeout[State](10*time.Second),
//		// ... other options
//	)
func SetStartTimeout[State any](tout time.Duration) GenSrvOpt[State] {
	return func(c *config[State]) {
		c.startTimeout = tout
	}
}

// RegisterInit registers the initialization handler called when the server starts.
//
// This is equivalent to implementing the init/1 callback in Erlang's gen_server.
// The handler is called synchronously during [Start], [StartLink], or [StartMonitor],
// and must complete before those functions return.
//
// Parameters:
//   - init: Function that initializes the server state
//
// The init function receives:
//   - self: The PID of the new GenServer process
//   - arg: The argument passed to Start/StartLink/StartMonitor
//
// The init function returns:
//   - State: The initial server state
//   - any: An optional continuation term (nil if not using continuations)
//   - error: An error to abort startup, or nil for success
//
// Special error handling:
//   - nil: Server starts successfully
//   - [exitreason.Ignore]: Server shuts down cleanly, caller receives [exitreason.Ignore]
//   - Other errors: Server terminates, error returned to caller
//
// Example:
//
//	gensrv.RegisterInit[MyState, Config](func(self erl.PID, cfg Config) (MyState, any, error) {
//		if cfg.Port == 0 {
//			return MyState{}, nil, fmt.Errorf("invalid port")
//		}
//		return MyState{Port: cfg.Port, Connections: 0}, nil, nil
//	})
func RegisterInit[State any, Arg any](init func(self erl.PID, arg Arg) (State, any, error)) GenSrvOpt[State] {
	return func(c *config[State]) {
		c.initFun = func(self erl.PID, a any) (genserver.InitResult[State], error) {
			msg := a.(Arg)
			s, c, err := init(self, msg)
			return genserver.InitResult[State]{Continue: c, State: s}, err
		}
	}
}

// RegisterCast registers a handler for asynchronous (fire-and-forget) messages.
//
// This is equivalent to implementing handle_cast/2 in Erlang's gen_server.
// Cast handlers process messages sent via [genserver.Cast]. The caller does not
// wait for a response - the message is delivered to the server's inbox and
// processed asynchronously.
//
// Parameters:
//   - matchType: A zero-value instance of the message type to match (e.g., Increment{})
//   - fn: The handler function to call when a matching message is received
//
// The handler function receives:
//   - self: The server's PID
//   - a: The cast message (type-asserted to Msg)
//   - state: The current server state
//
// The handler returns:
//   - newState: The updated server state
//   - continu: An optional continuation term to trigger a [RegisterContinue] handler
//   - err: An error that causes the server to terminate, or nil to continue
//
// Use Cast (vs Call) when:
//   - The caller doesn't need a response
//   - You want maximum throughput (no round-trip latency)
//   - You're implementing notifications or events
//
// Example:
//
//	gensrv.RegisterCast[State, Increment](Increment{}, func(self erl.PID, msg Increment, state State) (State, any, error) {
//		state.Count += msg.Amount
//		return state, nil, nil
//	})
func RegisterCast[State any, Msg any](matchType any, fn func(self erl.PID, a Msg, state State) (newState State, continu any, err error)) GenSrvOpt[State] {
	return func(c *config[State]) {
		termT := reflect.TypeOf(matchType)
		c.castFuns[termT] = func(self erl.PID, m any, s State) (newState State, continu any, err error) {
			msg := m.(Msg)
			return fn(self, msg, s)
		}
	}
}

// RegisterInfo registers a handler for system and informational messages.
//
// This is equivalent to implementing handle_info/2 in Erlang's gen_server.
// Info handlers process messages that arrive directly in the process inbox
// (via [erl.Send]) rather than through [genserver.Call] or [genserver.Cast].
//
// Common uses for Info handlers:
//   - [erl.DownMsg]: Monitor notifications when a watched process exits
//   - [erl.ExitMsg]: Exit signals from linked processes (when trapping exits)
//   - Timer messages from scheduled operations
//   - Messages from ports or external sources
//   - Any message sent directly via [erl.Send]
//
// Parameters:
//   - matchType: A zero-value instance of the message type to match
//   - fn: The handler function to call when a matching message is received
//
// The handler function receives:
//   - self: The server's PID
//   - a: The info message (type-asserted to Msg)
//   - state: The current server state
//
// The handler returns:
//   - newState: The updated server state
//   - continu: An optional continuation term to trigger a [RegisterContinue] handler
//   - err: An error that causes the server to terminate, or nil to continue
//
// Example handling monitor down messages:
//
//	gensrv.RegisterInfo[State, erl.DownMsg](erl.DownMsg{}, func(self erl.PID, msg erl.DownMsg, state State) (State, any, error) {
//		log.Printf("Monitored process %v exited: %v", msg.Pid, msg.Reason)
//		delete(state.Workers, msg.Pid)
//		return state, nil, nil
//	})
func RegisterInfo[State any, Msg any](matchType any, fn func(self erl.PID, a Msg, state State) (newState State, continu any, err error)) GenSrvOpt[State] {
	return func(c *config[State]) {
		termT := reflect.TypeOf(matchType)
		c.infoFuns[termT] = func(self erl.PID, m any, s State) (newState State, continu any, err error) {
			msg := m.(Msg)
			return fn(self, msg, s)
		}
	}
}

// RegisterCall registers a handler for synchronous request/response messages.
//
// This is equivalent to implementing handle_call/3 in Erlang's gen_server.
// Call handlers process requests sent via [genserver.Call]. The caller blocks
// until a response is sent or the call times out.
//
// Parameters:
//   - matchType: A zero-value instance of the message type to match (e.g., GetValue{})
//   - fn: The handler function to call when a matching request is received
//
// The handler function receives:
//   - self: The server's PID
//   - request: The call request message (type-asserted to Msg)
//   - from: Identifies the caller; use with [genserver.Reply] for deferred responses
//   - state: The current server state
//
// The handler returns:
//   - result: A [genserver.CallResult] containing the reply and updated state
//   - err: An error that causes the server to terminate, or nil to continue
//
// The [genserver.CallResult] fields:
//   - Msg: The response to send to the caller (ignored if NoReply is true)
//   - State: The updated server state
//   - Continue: An optional continuation term
//   - NoReply: Set to true if using [genserver.Reply] for deferred response
//
// Deadlock safety: [genserver.Call] prevents a process from calling itself,
// returning an error immediately if attempted.
//
// Example:
//
//	gensrv.RegisterCall[State, GetCount](GetCount{}, func(self erl.PID, req GetCount, from genserver.From, state State) (genserver.CallResult[State], error) {
//		return genserver.CallResult[State]{
//			Msg:   state.Count,
//			State: state,
//		}, nil
//	})
func RegisterCall[State any, Msg any](matchType any, fn func(self erl.PID, request Msg, from genserver.From, state State) (result genserver.CallResult[State], err error)) GenSrvOpt[State] {
	return func(c *config[State]) {
		termT := reflect.TypeOf(matchType)
		c.callFuns[termT] = func(self erl.PID, m any, f genserver.From, s State) (result genserver.CallResult[State], err error) {
			msg := m.(Msg)
			return fn(self, msg, f, s)
		}
	}
}

// RegisterContinue registers a handler for continuation (deferred processing) messages.
//
// This is equivalent to implementing handle_continue/2 in Erlang's gen_server.
// Continuation handlers are triggered when another handler (Init, Call, Cast, Info,
// or another Continue) returns a non-nil continuation term. The continuation is
// processed immediately after the triggering handler completes, before any new
// messages from the inbox.
//
// Continuations are useful for:
//   - Returning from Init quickly (unblocking [StartLink]) while performing
//     additional setup work before processing messages
//   - Returning a Call reply to the caller before doing post-processing work
//   - Implementing state machines with explicit transitions
//   - Code reuse when the same logic applies to multiple handler types
//
// Parameters:
//   - matchType: A zero-value instance of the continuation type to match
//   - fn: The handler function to call when a matching continuation is triggered
//
// The handler function receives:
//   - self: The server's PID
//   - cont: The continuation term (type-asserted to Msg)
//   - state: The current server state
//
// The handler returns:
//   - newState: The updated server state
//   - continu: Another continuation term to chain processing, or nil to stop
//   - err: An error that causes the server to terminate, or nil to continue
//
// Example returning from Init quickly while deferring expensive setup:
//
//	type LoadCache struct{}
//
//	gensrv.RegisterInit[State, Config](func(self erl.PID, cfg Config) (State, any, error) {
//		// Return immediately so StartLink unblocks, but trigger cache loading
//		return State{Config: cfg}, LoadCache{}, nil
//	})
//
//	gensrv.RegisterContinue[State, LoadCache](LoadCache{}, func(self erl.PID, _ LoadCache, state State) (State, any, error) {
//		// This runs before any Call/Cast/Info messages are processed
//		state.Cache = loadExpensiveCache(state.Config)
//		return state, nil, nil
//	})
func RegisterContinue[State any, Msg any](matchType any, fn func(self erl.PID, cont Msg, state State) (newState State, continu any, err error)) GenSrvOpt[State] {
	return func(c *config[State]) {
		termT := reflect.TypeOf(matchType)
		c.continueFuns[termT] = func(self erl.PID, m any, s State) (newState State, continu any, err error) {
			msg := m.(Msg)
			return fn(self, msg, s)
		}
	}
}

// RegisterTerminate registers a cleanup handler called when the server terminates.
//
// This is equivalent to implementing terminate/2 in Erlang's gen_server.
//
// The terminate handler is invoked in the following scenarios:
//  1. When [genserver.Stop] is called on the GenServer process
//  2. When any handler (Call, Cast, Info, Continue) panics or returns an error
//  3. When the GenServer is trapping exits (via [erl.ProcessFlag] with [erl.TrapExit])
//     and receives an [erl.ExitMsg] from its parent process or supervisor
//
// Note: If the GenServer is NOT trapping exits, exit signals from linked processes
// cause immediate termination without calling the terminate handler.
//
// Parameters:
//   - terminate: The cleanup function to call before the server exits
//
// The terminate function receives:
//   - self: The server's PID
//   - reason: The error that caused termination (e.g., [exitreason.Normal], [exitreason.Shutdown])
//   - state: The final server state
//
// Important notes:
//   - Terminate cannot prevent the server from exiting
//   - Panics in Terminate are caught at the Process level (cleanup still happens)
//   - Use Terminate for releasing resources, closing connections, saving state, etc.
//   - The return value is ignored; this is a cleanup-only callback
//
// Example:
//
//	gensrv.RegisterTerminate[State](func(self erl.PID, reason error, state State) {
//		log.Printf("Server %v terminating: %v", self, reason)
//		if state.DB != nil {
//			state.DB.Close()
//		}
//		if state.File != nil {
//			state.File.Close()
//		}
//	})
func RegisterTerminate[State any](terminate func(self erl.PID, reason error, state State)) GenSrvOpt[State] {
	return func(c *config[State]) {
		c.terminateFun = terminate
	}
}
