// Package gensrv provides a registration-based GenServer implementation.
//
// This is a higher-level alternative to the [genserver] package that allows you to
// register handlers for specific message types using functional options, rather
// than implementing the full GenServer interface. It builds on top of the underlying
// genserver package and offers a type-safe and ergonomic API using Go's generics.
//
// This is conceptually similar to Erlang/OTP's gen_server, but with a more Go-idiomatic
// registration-based API rather than requiring a full callback module implementation.
//
// # When to Use gensrv vs genserver
//
// Use gensrv when:
//   - You have discrete message types that each need their own handler
//   - You prefer a functional options pattern over implementing interfaces
//   - Your server logic is straightforward and doesn't need custom dispatch
//
// Use [genserver] when:
//   - You need full control over the callback interface
//   - You want to implement custom message routing in HandleCall/HandleCast
//   - You're porting existing Erlang code that uses the gen_server callbacks directly
//
// # Basic Usage
//
// Define your state and message types, then use the Register* functions to configure handlers:
//
//	// Define the server state
//	type CounterState struct {
//		Value int
//	}
//
//	// Define message types
//	type Increment struct{}
//	type GetValue struct{}
//
//	// Starting the server with registered handlers
//	pid, err := gensrv.StartLink[CounterState](
//		self, nil,
//		gensrv.SetName[CounterState]("counter"),
//		gensrv.RegisterInit[CounterState, any](func(self erl.PID, arg any) (CounterState, any, error) {
//			return CounterState{Value: 0}, nil, nil
//		}),
//		gensrv.RegisterCast[CounterState, Increment](Increment{}, func(self erl.PID, msg Increment, state CounterState) (CounterState, any, error) {
//			state.Value++
//			return state, nil, nil
//		}),
//		gensrv.RegisterCall[CounterState, GetValue](GetValue{}, func(self erl.PID, msg GetValue, from genserver.From, state CounterState) (genserver.CallResult[CounterState], error) {
//			return genserver.CallResult[CounterState]{Msg: state.Value, State: state}, nil
//		}),
//		gensrv.RegisterTerminate[CounterState](func(self erl.PID, reason error, state CounterState) {
//			log.Printf("Counter terminating with value %d: %v", state.Value, reason)
//		}),
//	)
//
// # Sending Messages
//
// Use [genserver.Call] for synchronous requests and [genserver.Cast] for asynchronous messages:
//
//	// Asynchronous increment (fire-and-forget)
//	genserver.Cast(counterPID, Increment{})
//
//	// Synchronous value retrieval
//	value, err := genserver.Call(self, counterPID, GetValue{}, 5*time.Second)
//
// # Handler Types
//
// The package supports five types of handlers:
//
//   - [RegisterInit]: Called once when the server starts to initialize state
//   - [RegisterCall]: Handles synchronous requests that expect a response
//   - [RegisterCast]: Handles asynchronous messages (fire-and-forget)
//   - [RegisterInfo]: Handles system messages and other non-API messages
//   - [RegisterContinue]: Handles deferred processing triggered by other handlers
//   - [RegisterTerminate]: Called when the server is shutting down for cleanup
//
// # The From Parameter in Call Handlers
//
// Call handlers receive a [genserver.From] parameter that identifies the caller.
// In most cases, you return the reply in [genserver.CallResult.Msg] and don't use From directly.
// However, From is useful when you need to:
//
//   - Reply before the handler returns (to unblock the caller early)
//   - Reply from a different callback (e.g., after receiving async data in an Info handler)
//   - Spawn a goroutine to compute the reply asynchronously
//
// To send a deferred reply, use [genserver.Reply]:
//
//	gensrv.RegisterCall[State, SlowQuery](SlowQuery{}, func(self erl.PID, req SlowQuery, from genserver.From, state State) (genserver.CallResult[State], error) {
//		// Start async work and reply later
//		go func() {
//			result := expensiveComputation(req)
//			genserver.Reply(from, result)
//		}()
//		// Return NoReply to indicate we'll reply later
//		return genserver.CallResult[State]{NoReply: true, State: state}, nil
//	})
//
// # Continuations
//
// Continuations allow you to return from a handler while deferring additional work
// that runs before any new messages are processed. Return a non-nil continuation
// term from any handler to trigger the matching [RegisterContinue] handler.
//
// Continuations are useful for:
//   - Returning from Init quickly (unblocking [StartLink]) while performing setup
//   - Returning a Call reply to the caller before doing post-processing
//   - Implementing state machines with explicit transitions
//   - Code reuse when the same logic applies to multiple handler types
//
// Example deferring expensive initialization:
//
//	type LoadCache struct{}
//
//	gensrv.RegisterInit[State, Config](func(self erl.PID, cfg Config) (State, any, error) {
//		// Return immediately so StartLink unblocks
//		return State{Config: cfg}, LoadCache{}, nil
//	})
//
//	gensrv.RegisterContinue[State, LoadCache](LoadCache{}, func(self erl.PID, _ LoadCache, state State) (State, any, error) {
//		// Runs before any messages are processed
//		state.Cache = loadExpensiveCache(state.Config)
//		return state, nil, nil
//	})
//
// # Panic Recovery
//
// All panics in registered handlers are automatically caught at the Process level.
// You do not need to add defer/recover in your handler implementations. When a
// panic occurs in any registered callback (Init, Call, Cast, Info, Continue, Terminate):
//
//   - The panic is caught and converted to [exitreason.Exception] with stack trace
//   - The process exits cleanly (all links and monitors are notified)
//   - Linked supervisors receive the exit signal and can restart the process
//   - [genserver.Call] operations return an error rather than hanging indefinitely
//   - The restarted process begins with fresh state from Init
//
// This ensures that supervision trees can restart processes from clean state
// after a panic, maintaining the fault-tolerance guarantees of the system.
// See the [genserver] package documentation for more details on panic recovery behavior.
//
// # Error Handling
//
// Errors returned from handlers cause the server to terminate:
//
//   - [RegisterInit] error: Server fails to start, error returned to caller
//   - [RegisterInit] returning [exitreason.Ignore]: Server shuts down normally, no error
//   - [RegisterCall]/[RegisterCast]/[RegisterInfo] error: Server terminates with that error
//   - [RegisterTerminate]: Called with the termination reason for cleanup
//
// For expected errors in Call handlers, return the error in [genserver.CallResult.Msg]
// rather than as an error, so the server continues running:
//
//	return genserver.CallResult[State]{Msg: fmt.Errorf("invalid input"), State: state}, nil
package gensrv
