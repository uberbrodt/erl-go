// Package gensrv provides a generic server pattern implementation inspired by Erlang/OTP GenServer.
// It builds on top of the underlying genserver package and offers a type-safe and ergonomic API
// using Go's generics.
//
// The GenServer pattern facilitates the creation of server processes that maintain state and
// handle different types of messages in a structured way. This package provides a simplified API
// for implementing servers that can:
//   - Be initialized with custom state
//   - Handle asynchronous messages (casts)
//   - Handle synchronous requests (calls)
//   - Process informational messages
//   - Manage continuations for deferred processing
//   - Perform cleanup on termination
//
// Example usage with named functions:
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
//	// Init function to create initial state
//	func InitCounter(self erl.PID, arg any) (CounterState, any, error) {
//		return CounterState{Value: 0}, nil, nil
//	}
//
//	// Handler for increment messages (cast)
//	func HandleIncrement(self erl.PID, msg Increment, state CounterState) (CounterState, any, error) {
//		state.Value++
//		return state, nil, nil
//	}
//
//	// Handler for value retrieval (call)
//	func HandleGetValue(self erl.PID, msg GetValue, from genserver.From, state CounterState) (genserver.CallResult[CounterState], error) {
//		return genserver.CallResult[CounterState]{
//			Msg:   state.Value,
//			State: state,
//		}, nil
//	}
//
//	// Termination handler
//	func HandleTerminate(self erl.PID, reason error, state CounterState) {
//		log.Printf("Counter server terminating with value %d: %v", state.Value, reason)
//	}
//
//	// Starting the server
//	pid, err := gensrv.Start[CounterState](
//		self, nil,
//		gensrv.SetName[CounterState]("counter"),
//		gensrv.RegisterInit[CounterState, any](InitCounter),
//		gensrv.RegisterCast[CounterState, Increment](Increment{}, HandleIncrement),
//		gensrv.RegisterCall[CounterState, GetValue](GetValue{}, HandleGetValue),
//		gensrv.RegisterTerminate[CounterState](HandleTerminate),
//	)
package gensrv
