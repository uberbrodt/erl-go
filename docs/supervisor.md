
# Supervisors

A supervisor is a process that supervises other processes, which we refer to as child processes. Supervisors are used to build a hierarchical process structure called a supervision tree. A supervision tree is a great way to structure a fault-tolerant application.

## Core Concepts

### Supervisor

A supervisor is a specialized `genserver` that manages a dynamic list of child processes. It's responsible for starting, stopping, and restarting its children according to its configured restart strategy.

### ChildSpec

A `ChildSpec` is a struct that defines how a supervisor should manage a child process. It includes:

- **ID:** A unique identifier for the child.
- **Start:** A function that the supervisor calls to start the child process. This function must link the child to the supervisor.
- **Restart:** The restart strategy for this specific child (`Permanent`, `Temporary`, `Transient`).
- **Shutdown:** How the child should be terminated.
- **Type:** The type of child (`Worker` or `Supervisor`).

### Restart Strategies

A supervisor's restart strategy determines which child processes are restarted when one of them terminates.

- **OneForOne:** If a child process terminates, only that child is restarted.
- **OneForAll:** If a child process terminates, all other child processes are terminated and then all of them (including the terminated one) are restarted.
- **RestForOne:** If a child process terminates, the rest of the child processes (that is, the children that were started after the terminated child) are terminated. Then the terminated child and the rest of the children are restarted.

### Shutdown Options

When a supervisor terminates a child, it can do so in a few ways:

- **Timeout:** The supervisor sends an exit signal to the child and waits for a configurable amount of time for the child to terminate. If the child doesn't terminate within the timeout, the supervisor sends a `kill` signal.
- **BrutalKill:** The supervisor immediately sends a `kill` signal to the child.
- **Infinity:** The supervisor sends an exit signal and waits indefinitely for the child to terminate.

### Supervisor Flags

Supervisor flags control the restart frequency of child processes.

- **Period:** The time window in seconds during which restarts are counted.
- **Intensity:** The maximum number of restarts allowed within the `Period`. If the number of restarts exceeds the `Intensity` within the `Period`, the supervisor terminates itself and all its children.

## Usage Example

Here's an example of how to start a simple supervisor with one child process.

```go
package main

import (
	"fmt"
	"time"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/genserver"
	"github.com/uberbrodt/erl-go/erl/supervisor"
)

// A simple worker server
type MyServer struct{}

func (s MyServer) Init(self erl.PID, args any) (genserver.InitResult[any], error) {
	fmt.Println("MyServer starting")
	return genserver.InitResult[any]{State: nil}, nil
}
func (s MyServer) HandleCall(self erl.PID, request any, from genserver.From, state any) (genserver.CallResult[any], error) {
	return genserver.CallResult[any]{Msg: "pong", State: state}, nil
}
func (s MyServer) HandleCast(self erl.PID, arg any, state any) (genserver.CastResult[any], error) {
	return genserver.CastResult[any]{State: state}, nil
}
func (s MyServer) HandleInfo(self erl.PID, request any, state any) (genserver.InfoResult[any], error) {
	return genserver.InfoResult[any]{State: state}, nil
}
func (s MyServer) HandleContinue(self erl.PID, continuation any, state any) (any, any, error) {
	return state, nil, nil
}
func (s MyServer) Terminate(self erl.PID, arg error, state any) {}

func main() {
	// The root process for our application
	rootPID := erl.Spawn(func(self erl.PID, inbox <-chan any) error {
		// Define a child spec for our worker server
		child := supervisor.NewChildSpec(
			"my_server",
			func(sup erl.PID) (erl.PID, error) {
				return genserver.StartLink[any](sup, MyServer{}, nil)
			},
		)

		// Configure the supervisor
		supFlags := supervisor.NewSupFlags(supervisor.SetStrategy(supervisor.OneForOne))
		children := []supervisor.ChildSpec{child}

		// Start the supervisor
		supPID, err := supervisor.StartDefaultLink(self, children, supFlags)
		if err != nil {
			fmt.Printf("Error starting supervisor: %v\n", err)
			return
		}

		fmt.Printf("Supervisor started with PID: %v\n", supPID)

		// Keep the root process alive
		for {
			time.Sleep(1 * time.Second)
		}
		return nil
	})

	// Let the supervisor run for a bit
	time.Sleep(5 * time.Second)
	erl.Exit(erl.RootPID, rootPID, "done")
}
```

## Public API

### `StartDefaultLink(self erl.PID, children []ChildSpec, supFlags SupFlagsS, optFuns ...LinkOpts) (erl.PID, error)`

Starts a supervisor with a default callback handler. This is the simplest way to start a supervisor.

### `StartLink(self erl.PID, callback Supervisor, args any, optFuns ...LinkOpts) (erl.PID, error)`

Starts a supervisor with a custom callback module. This allows you to dynamically define the children a supervisor should have.

### `NewChildSpec(id string, start StartFunSpec, opts ...ChildSpecOpt) ChildSpec`

Creates a new `ChildSpec`.

- **`SetRestart(restart Restart)`:** Sets the restart strategy for the child.
- **`SetShutdown(shutdown ShutdownOpt)`:** Sets the shutdown options for the child.
- **`SetChildType(t ChildType)`:** Sets the child type.

### `NewSupFlags(flags ...SupFlag) SupFlagsS`

Creates a new `SupFlagsS` struct.

- **`SetStrategy(strategy Strategy)`:** Sets the supervisor's restart strategy.
- **`SetPeriod(period int)`:** Sets the restart period.
- **`SetIntensity(intensity int)`:** Sets the restart intensity.
