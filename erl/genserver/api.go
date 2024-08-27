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
	switch start {
	case noLink:
		pid = erl.Spawn(gs)

	case monitor:
		pid, monref = erl.SpawnMonitor(self, gs)

	case link:
		pid = erl.SpawnLink(self, gs)
	}

	// pid := spawnFun(self)

	select {
	case ack := <-initAckChan:
		erl.DebugPrintf("GenServer[%v] received initAck: %+v", pid, ack)
		if ack.ignore {
			return startRet{pid: pid, err: exitreason.Ignore, monref: monref}
		}
		// XXX: hack to wait for ExitMsgs to be delivered. I think the right way to do
		// this would be to create a separate process that starts the genserver
		// consumes exitmsgs if there's an error, and returns it.
		// the caveat is that the starter needs to swap the GenServer parent pid with the
		// [self] pid from this function so Supervisors work right.
		if ack.err != nil {
			for {
				if !erl.IsAlive(pid) {
					return startRet{pid: pid, err: ack.err, monref: monref}
				}
			}
		}
		return startRet{pid: pid, err: ack.err, monref: monref}

	case <-time.After(finalOpts.GetStartTimeout()):
		// TODO: need to wait until exit messages are delivered to know if the process
		// name was released
		erl.Exit(self, pid, exitreason.Kill)
		return startRet{pid: pid, err: exitreason.Timeout, monref: monref}
	}
}
