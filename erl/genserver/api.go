/*
Package genserver implements the GenServer behavior from Erlang/OTP.

A GenServer is a process that implements a standard set of callback functions
to handle synchronous requests (Call), asynchronous messages (Cast), and
information messages (Info). GenServers are the foundation for building
stateful, concurrent services in the erl-go framework.

# Basic Usage

Implement the GenServer[State] interface:

	type MyServer struct{}
	type MyState struct { Count int }

	func (s MyServer) Init(self erl.PID, args any) (genserver.InitResult[MyState], error) {
		return genserver.InitResult[MyState]{State: MyState{Count: 0}}, nil
	}

	func (s MyServer) HandleCall(self erl.PID, request any, from genserver.From, state MyState) (genserver.CallResult[MyState], error) {
		// Handle synchronous requests
		return genserver.CallResult[MyState]{Msg: state.Count, State: state}, nil
	}

	// ... implement HandleCast, HandleInfo, HandleContinue, Terminate

Then start the server:

	pid, err := genserver.StartLink[MyState](self, MyServer{}, nil)

# Panic Recovery

All panics in GenServer callbacks are automatically caught at the Process level.
You do not need to add defer/recover in your callback implementations. When a
panic occurs:

  - The panic is caught and converted to exitreason.Exception
  - The process exits cleanly (links and monitors are notified)
  - Linked supervisors receive the exit signal and can restart the process
  - Call operations return an error rather than hanging indefinitely

This ensures that supervision trees can restart processes from clean state
after a panic, maintaining the fault-tolerance guarantees of the system.
*/
package genserver

import (
	"errors"
	"fmt"
	"time"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
	"github.com/uberbrodt/erl-go/erl/timeout"
)

// StartLink starts a GenServer process linked to the calling process.
//
// This is equivalent to Erlang's gen_server:start_link/3,4. The new process
// is atomically linked to the caller, meaning if either process exits abnormally,
// the other will receive an exit signal. This is the recommended way to start
// GenServers under a supervisor.
//
// Parameters:
//   - self: The PID of the calling process (required for linking)
//   - callbackStruct: Implementation of the [GenServer] interface
//   - args: Arguments passed to the Init callback
//   - opts: Optional configuration (see [StartOpt])
//
// The function blocks until Init completes. If Init returns an error or
// [exitreason.Ignore], the process terminates and that error is returned.
// If Init times out (configurable via [SetStartTimeout]), [exitreason.Timeout]
// is returned and the process is killed.
//
// See [StartError] for what type of error is returned.
func StartLink[STATE any](self erl.PID, callbackStruct GenServer[STATE], args any, opts ...StartOpt) (erl.PID, error) {
	result := doStart(self, link, callbackStruct, args, opts...)
	return result.pid, result.err
}

// StartMonitor starts a GenServer process and monitors it from the calling process.
//
// This is equivalent to Erlang's gen_server:start_monitor/3,4. Unlike [StartLink],
// a monitor is one-way: if the GenServer exits, the caller receives a [erl.DownMsg]
// but is not killed. This is useful when you want to be notified of process death
// without being affected by it.
//
// Parameters:
//   - self: The PID of the calling process (required for monitoring)
//   - callbackStruct: Implementation of the [GenServer] interface
//   - args: Arguments passed to the Init callback
//   - opts: Optional configuration (see [StartOpt])
//
// Returns the PID, a monitor reference (for use with [erl.Demonitor]), and any error.
// The function blocks until Init completes.
func StartMonitor[STATE any](self erl.PID, callbackStruct GenServer[STATE], args any, opts ...StartOpt) (erl.PID, erl.Ref, error) {
	result := doStart(self, monitor, callbackStruct, args, opts...)
	return result.pid, result.monref, result.err
}

// Start starts a GenServer process without linking or monitoring.
//
// This is equivalent to Erlang's gen_server:start/3,4. The new process runs
// independently of the caller - neither will be notified if the other exits.
// Use this when you need a standalone GenServer not managed by a supervisor.
//
// Parameters:
//   - self: The PID of the calling process (required for Init synchronization)
//   - callbackStruct: Implementation of the [GenServer] interface
//   - args: Arguments passed to the Init callback
//   - opts: Optional configuration (see [StartOpt])
//
// The self PID is required because the GenServer will notify it when Init is
// completed. If the parent is trapping exits, Terminate will also be called
// when the parent exits.
//
// The function blocks until Init completes. For supervised processes, use
// [StartLink] instead.
func Start[STATE any](self erl.PID, callbackStruct GenServer[STATE], args any, opts ...StartOpt) (erl.PID, error) {
	result := doStart(self, noLink, callbackStruct, args, opts...)
	return result.pid, result.err
}

// Reply sends a response to a Call request from within a HandleCall callback.
//
// This is equivalent to Erlang's gen_server:reply/2. Use this when you need to
// defer sending a reply to a Call request, for example when the response depends
// on an asynchronous operation.
//
// In most cases, you can simply return the reply in [CallResult.Msg] from HandleCall.
// Use Reply explicitly when:
//   - You want to reply before HandleCall returns (to unblock the caller sooner)
//   - You need to reply from a different callback (e.g., HandleInfo after receiving
//     data from an external source)
//   - You want to send the reply from a spawned process
//
// Parameters:
//   - client: The [From] value passed to HandleCall, identifying the caller
//   - reply: The response to send back to the caller
//
// Note: Each Call request should receive exactly one reply. Sending multiple
// replies or no reply will cause undefined behavior for the caller.
func Reply(client From, reply any) {
	erl.Send(client.caller, CallReply{Status: OK, Term: reply})
}

// Cast sends an asynchronous request to a GenServer.
//
// This is equivalent to Erlang's gen_server:cast/2. The request is sent to the
// GenServer's inbox and will be processed by the HandleCast callback. Cast is
// fire-and-forget: it returns immediately without waiting for the message to be
// processed or for any response.
//
// The main difference between a Cast and an [erl.Send] is the ability to resolve process names and the possible error returns.
// Otherwise they are the same.
//
// Parameters:
//   - gensrv: The destination GenServer (PID or registered name)
//   - request: The message to send, which will be passed to HandleCast
//
// Returns [exitreason.NoProc] if the destination cannot be resolved to a valid PID.
// Note that a nil error does not guarantee delivery - the target process may exit
// before processing the message.
//
// Use Cast for:
//   - Notifications that don't require acknowledgment
//   - Performance-critical operations where you can't wait for a response
//   - Avoiding deadlocks in bidirectional communication patterns
//
// For operations that need a response, use [Call] instead.
func Cast(gensrv erl.Dest, request any) error {
	pid, err := gensrv.ResolvePID()
	if err != nil {
		return exitreason.NoProc
	}
	erl.Send(pid, CastRequest{Msg: request})
	return nil
}

// Call makes a synchronous request to a GenServer and waits for a response.
//
// This is equivalent to Erlang's gen_server:call/2,3. The request is sent to
// the GenServer and the caller blocks until a response is received or the
// timeout expires. The request is processed by the GenServer's HandleCall
// callback.
//
// Parameters:
//   - self: The PID of the calling process (used for deadlock detection)
//   - gensrv: The destination GenServer (PID or registered name)
//   - request: The message to send, which will be passed to HandleCall
//   - timeout: Maximum time to wait for a response
//
// Returns the reply from HandleCall and nil error on success.
//
// Possible errors:
//   - [exitreason.NoProc]: The destination cannot be resolved or doesn't exist
//   - [exitreason.Timeout]: No response received within the timeout period
//   - [exitreason.Stopped]: The GenServer stopped without sending a reply
//   - [exitreason.Exception]: Calling self (deadlock) or other unexpected errors
//
// Call is safe from deadlocks because it uses an intermediate process to manage
// the request/response cycle. However, calling yourself will return an error
// immediately to prevent obvious deadlocks.
//
// For fire-and-forget operations, use [Cast] instead.
func Call(self erl.PID, gensrv erl.Dest, request any, timeout time.Duration) (any, error) {
	resp := make(chan any)

	pid, err := gensrv.ResolvePID()
	if err != nil {
		return nil, exitreason.NoProc
	}

	// calling yourself is a deadlock
	if self == gensrv {
		return nil, exitreason.Exception(fmt.Errorf("cannot call self"))
	}

	erl.Spawn(&genCaller{out: resp, gensrv: pid, tout: timeout, request: request})

	select {
	case msg := <-resp:
		switch msgT := msg.(type) {
		case CallReply:

			if msgT.Status == Stopped {
				return nil, exitreason.Stopped
			} else if msgT.Status == Other {
				return nil, exitreason.Exception(fmt.Errorf("Got error: %v", msgT))
			} else if msgT.Status == Timeout {
				return nil, exitreason.Timeout
			}
			return msgT.Term, nil
		default:
			return nil, exitreason.Exception(fmt.Errorf("received something other than a CallReply from genCaller: %+v", msg))
		}
	case <-time.After(timeout):
		return nil, exitreason.Timeout
	}
}

// Stop requests a GenServer to terminate and waits for it to exit.
//
// This is equivalent to Erlang's gen_server:stop/1,3. The function sends a
// stop request to the GenServer, which triggers its Terminate callback, and
// then waits for the process to exit.
//
// Parameters:
//   - self: The PID of the calling process (required)
//   - gensrv: The destination GenServer (PID or registered name)
//   - opts: Optional configuration (see [ExitOpt])
//
// Options:
//   - [StopTimeout]: Maximum time to wait for shutdown (default: infinity)
//   - [StopReason]: Exit reason to pass to Terminate (default: [exitreason.Normal])
//
// Returns nil if the GenServer exits with the expected reason. Otherwise returns:
//   - [exitreason.NoProc]: The destination doesn't exist or is already dead
//   - [exitreason.Timeout]: The GenServer didn't exit within the timeout
//   - Other [exitreason.S]: The GenServer exited with a different reason
//
// Example:
//
//	// Stop with 5 second timeout
//	err := genserver.Stop(self, serverPID, genserver.StopTimeout(5*time.Second))
//
//	// Stop with custom reason
//	err := genserver.Stop(self, serverPID, genserver.StopReason(exitreason.Shutdown(nil)))
func Stop(self erl.PID, gensrv erl.Dest, opts ...ExitOpt) error {
	myOpts := exitOptS{
		tout:       timeout.Infinity,
		exitReason: exitreason.Normal,
	}
	for _, opt := range opts {
		myOpts = opt(myOpts)
	}
	if self.IsNil() {
		return exitreason.Exception(fmt.Errorf("self/parent pid cannot be undefined"))
	}

	reply := make(chan *exitreason.S)

	// default exit reason is normal
	exitReason := exitreason.Normal

	if myOpts.exitReason != nil {
		exitReason = myOpts.exitReason
	}

	gensrvPID, err := gensrv.ResolvePID()
	if err != nil {
		return fmt.Errorf("%w detail: %s", exitreason.NoProc, err)
	}

	if !erl.IsAlive(gensrvPID) {
		return exitreason.NoProc
	}

	erl.Spawn(&genStopper{out: reply, caller: self, gensrv: gensrvPID, tout: myOpts.tout, exitReason: exitReason})

	var exit *exitreason.S

	select {
	case e := <-reply:
		exit = e

	case <-time.After(myOpts.tout):
		exit = exitreason.Timeout
	}

	if errors.Is(exit, exitReason) {
		return nil
	}

	return exit
}

// exitOptS holds configuration options for the Stop function.
type exitOptS struct {
	tout       time.Duration
	exitReason *exitreason.S
}

// ExitOpt is a functional option for configuring the [Stop] function.
type ExitOpt func(opts exitOptS) exitOptS

// StopTimeout sets the maximum time to wait for a GenServer to terminate.
//
// If the GenServer doesn't exit within this duration, [Stop] returns
// [exitreason.Timeout]. The default is [timeout.Infinity], meaning it
// will wait indefinitely.
//
// Example:
//
//	genserver.Stop(self, pid, genserver.StopTimeout(5*time.Second))
func StopTimeout(tout time.Duration) ExitOpt {
	return func(opts exitOptS) exitOptS {
		opts.tout = tout
		return opts
	}
}

// StopReason sets the exit reason to pass to the GenServer's Terminate callback.
//
// The default is [exitreason.Normal]. Common alternatives include:
//   - [exitreason.Shutdown]: For controlled shutdown (e.g., by supervisor)
//   - [exitreason.Kill]: For forced termination (Terminate is not called)
//
// Example:
//
//	genserver.Stop(self, pid, genserver.StopReason(exitreason.Shutdown(nil)))
func StopReason(e *exitreason.S) ExitOpt {
	return func(opts exitOptS) exitOptS {
		opts.exitReason = e
		return opts
	}
}

// CallReturnStatus indicates the outcome of a [Call] operation.
// This is used internally in [CallReply] to communicate the result status.
type CallReturnStatus string

const (
	// OK indicates the call completed successfully with a reply.
	OK CallReturnStatus = "ok"
	// NoProc indicates the target GenServer doesn't exist or can't be resolved.
	NoProc CallReturnStatus = "noproc"
	// Timeout indicates the call timed out waiting for a response.
	Timeout CallReturnStatus = "timeout"
	// CallingSelf indicates an attempt to call the same process (deadlock prevention).
	CallingSelf CallReturnStatus = "calling_self"
	// Shutdown indicates the supervisor stopped the GenServer during the call.
	Shutdown CallReturnStatus = "shutdown"
	// Stopped indicates the GenServer returned Stop without sending a reply.
	Stopped CallReturnStatus = "normal_shutdown"
	// Other indicates an unhandled error occurred.
	Other CallReturnStatus = "other"
)

// CallReply is the response message returned from a [Call] operation.
//
// This is an internal message type used by the genCaller process to communicate
// results back to the caller. It may appear in process inboxes when using
// [erltest.TestReceiver] for testing.
//
// Fields:
//   - Status: The outcome of the call (see [CallReturnStatus])
//   - Term: The reply value from HandleCall (only valid when Status is [OK])
type CallReply struct {
	Status CallReturnStatus
	Term   any
}

// startRet holds the return values from doStart.
type startRet struct {
	pid    erl.PID
	monref erl.Ref
	err    error
}

// startType specifies the relationship between caller and started GenServer.
type startType string

const (
	noLink  startType = "nolink"  // No link or monitor
	monitor startType = "monitor" // One-way monitor from caller to GenServer
	link    startType = "link"    // Bi-directional link
)

// doStart is the internal implementation for Start, StartLink, and StartMonitor.
// It spawns the GenServer process, waits for Init to complete, and establishes
// the appropriate link/monitor relationship based on the start type.
func doStart[STATE any](self erl.PID, start startType, callbackStruct GenServer[STATE], args any, opts ...StartOpt) startRet {
	if self.IsNil() {
		return startRet{err: exitreason.Exception(fmt.Errorf("self/parent pid cannot be undefined"))}
	}
	finalOpts := DefaultOpts()

	for _, opt := range opts {
		finalOpts = opt(finalOpts)
	}
	initAckChan := make(chan initAck)

	gs := &GenServerS[STATE]{
		callback:    callbackStruct,
		opts:        finalOpts,
		args:        args,
		parent:      self,
		initAckChan: initAckChan,
	}
	var pid erl.PID
	var monref erl.Ref
	gensrvPIDChan := make(chan erl.PID)
	exitSignalsReceived := make(chan struct{})

	// our caller is a supervisor like process, so provide synchronous error messages
	// by using an interlocutor process to consume process exits
	if erl.TrappingExits(self) {
		starterPID := erl.Spawn(&genStarter[STATE]{
			gensrv: gs,
			parent: self,
			sig:    exitSignalsReceived,
			gsPID:  gensrvPIDChan,
			tout:   finalOpts.GetStartTimeout(),
		})

		pid = <-gensrvPIDChan

		select {
		case ack := <-initAckChan:
			erl.DebugPrintf("GenServer[%v] received initAck: %+v", pid, ack)
			if ack.ignore {
				<-exitSignalsReceived
				return startRet{pid: pid, err: exitreason.Ignore, monref: monref}
			}
			if ack.err != nil {
				// this channel is closed when the exit signals for the failed child are delivered.
				// this will mean certain resources like process names are released, so a supervisor
				// for instance could proceed to retry restarting.
				//
				// in the interest of not blocking a process for any reason, it's possible that the genStarter
				// times out waiting for exit signals. It will close this channel as well, so there's still the
				// possibility that a name or something else is not released. This should only be happening if there's
				// a badly behaved process that isn't reading from the msg inbox and/or monitoring it for closure.
				<-exitSignalsReceived
				return startRet{pid: pid, err: ack.err, monref: monref}
			}
			// init returned and it wasn't an error, NOW we link/monitor the caller to the new process
			switch start {
			case link:
				erl.Link(self, pid)

			case monitor:
				monref = erl.Monitor(self, pid)

			default:
				// nothing to do
			}

			// we successfully started, so close the genStarter
			erl.Send(starterPID, genStarterShutdown{})

			return startRet{pid: pid, err: ack.err, monref: monref}

		case <-time.After(finalOpts.GetStartTimeout()):
			// TODO: need to wait until exit messages are delivered to know if the process
			// name was released
			erl.Exit(self, pid, exitreason.Kill)
			return startRet{pid: pid, err: exitreason.Timeout, monref: monref}
		}

	} else {
		// process isn't trapping exits, so only will receive return value in the case of a spawn or monitor
		switch start {
		case link:
			pid = erl.SpawnLink(self, gs)

		case monitor:
			pid, monref = erl.SpawnMonitor(self, gs)

		default:
			pid = erl.Spawn(gs)

		}

		select {
		case ack := <-initAckChan:

			erl.DebugPrintf("GenServer[%v] received initAck: %+v", pid, ack)
			if ack.ignore {
				return startRet{pid: pid, err: exitreason.Ignore, monref: monref}
			}
			return startRet{pid: pid, err: ack.err, monref: monref}
		case <-time.After(finalOpts.GetStartTimeout()):
			erl.Exit(self, pid, exitreason.Kill)
			return startRet{pid: pid, err: exitreason.Timeout, monref: monref}
		}

	}

	// genStarter will start the process for us and listen for any exit signals
}
