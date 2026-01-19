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

// see [StartError] for what type of error is returned.
func StartLink[STATE any](self erl.PID, callbackStruct GenServer[STATE], args any, opts ...StartOpt) (erl.PID, error) {
	result := doStart(self, link, callbackStruct, args, opts...)
	return result.pid, result.err
}

func StartMonitor[STATE any](self erl.PID, callbackStruct GenServer[STATE], args any, opts ...StartOpt) (erl.PID, erl.Ref, error) {
	result := doStart(self, monitor, callbackStruct, args, opts...)
	return result.pid, result.monref, result.err
}

// Like [StartLink], but no link is created.

// The [self] PID is required because the [GenServer] will notify it when [Init] is
// completed and will also call [Terminate] if the parent exits and the GenServer is
// trapping exits.
func Start[STATE any](self erl.PID, callbackStruct GenServer[STATE], args any, opts ...StartOpt) (erl.PID, error) {
	result := doStart(self, noLink, callbackStruct, args, opts...)
	return result.pid, result.err
}

func Reply(client From, reply any) {
	erl.Send(client.caller, CallReply{Status: OK, Term: reply}) // genCallerReply{reply: result.Reply})
}

func Cast(gensrv erl.Dest, request any) error {
	pid, err := gensrv.ResolvePID()
	if err != nil {
		return exitreason.NoProc
	}
	erl.Send(pid, CastRequest{Msg: request})
	return nil
}

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

type exitOptS struct {
	tout       time.Duration
	exitReason *exitreason.S
}

type ExitOpt func(opts exitOptS) exitOptS

func StopTimeout(tout time.Duration) ExitOpt {
	return func(opts exitOptS) exitOptS {
		opts.tout = tout
		return opts
	}
}

func StopReason(e *exitreason.S) ExitOpt {
	return func(opts exitOptS) exitOptS {
		opts.exitReason = e
		return opts
	}
}

type CallReturnStatus string

const (
	OK          CallReturnStatus = "ok"
	NoProc      CallReturnStatus = "noproc"
	Timeout     CallReturnStatus = "timeout"
	CallingSelf CallReturnStatus = "calling_self"
	// supervisor stopped the GenServer
	Shutdown CallReturnStatus = "shutdown"
	// genserver returned [Stop] without a reply. There may be a reason
	Stopped CallReturnStatus = "normal_shutdown"
	// unhandled error happened.
	Other CallReturnStatus = "other"
)

// This is an intermediate response when invoking [Call]. Included here since it will
// appear in process inboxes and so needs to be matched in [erl.TestReceiver]
type CallReply struct {
	Status CallReturnStatus
	Term   any
}

type startRet struct {
	pid    erl.PID
	monref erl.Ref
	err    error
}

type startType string

const (
	noLink  startType = "nolink"
	monitor startType = "monitor"
	link    startType = "link"
)

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
