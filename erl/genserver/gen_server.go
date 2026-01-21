package genserver

import (
	"errors"
	"fmt"
	"log"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
)

type From struct {
	caller erl.PID
	mref   erl.Ref
}

// intermediate message sent to [GenServerS] from [Call]. Not normally seen unless
// the receiver process in [Call] is not a [GenServer]
type CallRequest struct {
	From From
	Msg  any
}

// intermediate message sent to [GenServerS] from [Cast]. Not normally seen unless
// the receiver process in [Cast] is not a [GenServer]
type CastRequest struct {
	Msg any
}

type initAck struct {
	ignore bool
	err    error
}

// stopRequest is an internal message sent by genStopper to request graceful termination.
// This triggers the Terminate callback before the process exits.
type stopRequest struct {
	reason *exitreason.S
}

type (
	// InitResult is returned from the Init callback to provide the initial state
	// and an optional continuation term.
	//
	// Fields:
	//   - State: The initial server state
	//   - Continue: If non-nil, triggers [GenServer.HandleContinue] before processing messages
	InitResult[STATE any] struct {
		State    STATE
		Continue any
	}

	// CallResult is returned from HandleCall to provide the reply, updated state,
	// and control flow options.
	//
	// Fields:
	//   - NoReply: If true, no reply is sent; use [Reply] to respond later
	//   - Msg: The reply to send to the caller (ignored if NoReply is true)
	//   - State: The updated server state
	//   - Continue: If non-nil, triggers [GenServer.HandleContinue] after the reply is sent
	//
	// Note: When Continue is set, the reply is sent to the caller before HandleContinue
	// runs, allowing the caller to unblock while post-processing occurs.
	CallResult[STATE any] struct {
		// if true, then no reply will be sent to the caller. The genserver should reply with [Reply]
		// at a later time, otherwise the caller will time out.
		NoReply bool
		// The reply that will be sent to the caller
		Msg any
		// The updated state of the GenServer
		State STATE
		// if not nil, will call [GenServer.HandleContinue] immediately after [GenServer.HandleCall] returns with this as the continuation
		Continue any
	}

	// CastResult is returned from HandleCast to provide the updated state
	// and an optional continuation term.
	//
	// Fields:
	//   - State: The updated server state
	//   - Continue: If non-nil, triggers [GenServer.HandleContinue] after HandleCast returns
	CastResult[STATE any] struct {
		State    STATE
		Continue any
	}

	// InfoResult is returned from HandleInfo to provide the updated state
	// and an optional continuation term.
	//
	// Fields:
	//   - State: The updated server state
	//   - Continue: If non-nil, triggers [GenServer.HandleContinue] after HandleInfo returns
	InfoResult[STATE any] struct {
		State    STATE
		Continue any
	}
)

// GenServer is the interface that must be implemented to create a GenServer process.
//
// This is equivalent to implementing the gen_server behavior callbacks in Erlang/OTP.
// All callbacks are invoked within the GenServer's process context.
type GenServer[STATE any] interface {
	// Init initializes the server state when the process starts.
	// This is equivalent to Erlang's init/1 callback.
	Init(self erl.PID, args any) (InitResult[STATE], error)

	// HandleCall processes synchronous requests sent via [Call].
	// This is equivalent to Erlang's handle_call/3 callback.
	HandleCall(self erl.PID, request any, from From, state STATE) (CallResult[STATE], error)

	// HandleCast processes asynchronous messages sent via [Cast].
	// This is equivalent to Erlang's handle_cast/2 callback.
	HandleCast(self erl.PID, request any, state STATE) (CastResult[STATE], error)

	// HandleInfo processes messages sent directly to the process inbox via [erl.Send],
	// as well as system messages like [erl.DownMsg] and [erl.ExitMsg].
	// This is equivalent to Erlang's handle_info/2 callback.
	HandleInfo(self erl.PID, msg any, state STATE) (InfoResult[STATE], error)

	// HandleContinue processes continuation terms returned by other callbacks.
	// This is equivalent to Erlang's handle_continue/2 callback.
	//
	// Continuations are useful for:
	//   - Returning from Init quickly (unblocking [StartLink]) while performing
	//     additional setup work before processing messages
	//   - Returning a Call reply to the caller before doing post-processing work
	//   - Implementing state machines with explicit transitions
	//   - Code reuse when the same logic applies to multiple handler types
	//
	// The continuation is processed immediately after the triggering callback completes,
	// before any new messages from the inbox. Returning a non-nil continueTerm will
	// chain into another HandleContinue call.
	HandleContinue(self erl.PID, continuation any, state STATE) (newState STATE, continueTerm any, err error)

	// Terminate is called when the server is about to exit.
	// This is equivalent to Erlang's terminate/2 callback.
	Terminate(self erl.PID, reason error, state STATE)
}

type GenServerS[STATE any] struct {
	callback       GenServer[STATE]
	state          STATE
	opts           StartOpts
	args           any
	parent         erl.PID
	initAckChan    chan<- initAck
	nameRegistered bool
}

func (gs *GenServerS[STATE]) unregisterName() {
	if gs.nameRegistered {
		erl.Unregister(gs.opts.GetName())
	}
}

func (gs *GenServerS[STATE]) Receive(self erl.PID, inbox <-chan any) error {
	// register if name is set
	if gs.opts.GetName() != "" {
		if err := erl.Register(gs.opts.GetName(), self); err != nil {
			gs.initAckChan <- initAck{err: err}
			return err
		}
		gs.nameRegistered = true
	}
	// initReturn, err := gs.callback.Init(self, gs.args)
	initReturn, err := gs.handleInit(self, gs.args)
	if err != nil {
		if errors.Is(err, exitreason.Ignore) {
			gs.unregisterName()
			gs.initAckChan <- initAck{ignore: true}
			return exitreason.Normal

		} else {
			// we unregister the name now, because while the underyling process will
			// unregister the name before the links/monitors are sent, the StartLink/Start call
			// could return before then.
			erl.DebugPrintf("GenServer[%v] returned an error from init Callback: %v", self, initReturn)
			gs.unregisterName()
			err = exitreason.Wrap(err)
			gs.initAckChan <- initAck{err: err}
			return err

		}
	}
	// we're all good, ack and head into main loop
	gs.initAckChan <- initAck{}
	gs.state = initReturn.State

	if initReturn.Continue != nil {
		s, err := gs.doContinue(self, initReturn.Continue, gs.state)
		if err != nil {
			return err
		}
		gs.state = s
	}

	for {
		msg, ok := <-inbox
		// process exit closes the channel and we'll get an any<nil> if we're not trapping exits.
		// links and montiors have already been handled so just ruturn nil to end this goroutine.
		if !ok {
			return nil
		}
		switch msgT := msg.(type) {
		case CallRequest:
			if stop, err := gs.handleCallRequest(self, msgT); stop {
				log.Printf("GenServer exited: %+v", err)
				return err
			}
		case CastRequest:
			if err := gs.handleCastRequest(self, msgT); err != nil {
				return err
			}
		case stopRequest:
			erl.DebugPrintf("GenServer[%v] received stopRequest with reason %v", self, msgT.reason)
			gs.callback.Terminate(self, msgT.reason, gs.state)
			return msgT.reason
		case erl.ExitMsg:
			erl.DebugPrintf("GenServer[%v] got ExitMsg with reason %s from process %+v", self, msgT.Reason, msgT.Proc)
			if msgT.Proc.Equals(gs.parent) {
				erl.DebugPrintf("GenServer[%v] ExitMsg is from parent link, terminating", self)
				gs.callback.Terminate(self, msgT.Reason, gs.state)
				return msgT.Reason
			} else {
				erl.DebugPrintf("GenServer[%v] ExitMsg is NOT from parent link, sending to HandleInfo callback", self)
				err := gs.handleInfoRequest(self, msg)
				if err != nil {
					return err
				}
			}
		default:
			err := gs.handleInfoRequest(self, msg)
			if err != nil {
				return err
			}
		}
	}
}

// panicToException converts a recovered panic value to an exitreason.Exception error.
func panicToException(r any) error {
	e, ok := r.(error)
	if !ok {
		return exitreason.Exception(fmt.Errorf("panic: %v", r))
	}
	if !exitreason.IsException(e) {
		return exitreason.Exception(e)
	}
	return e
}

func (gs *GenServerS[STATE]) handleInit(self erl.PID, msg any) (result InitResult[STATE], err error) {
	defer func() {
		if r := recover(); r != nil {
			err = panicToException(r)
		}
	}()
	result, err = gs.callback.Init(self, gs.args)
	return result, err
}

func (gs *GenServerS[STATE]) doContinue(self erl.PID, inCont any, inState STATE) (s STATE, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = panicToException(r)
			gs.callback.Terminate(self, err, gs.state)
		}
	}()

	s = inState
	cont := inCont
	var contErr error

	// handlers can return continues in a sort of chain, so follow it until
	// we get an error or a simple return
	for {
		s, cont, contErr = gs.callback.HandleContinue(self, cont, s)
		if contErr != nil {
			return s, contErr
		}
		if cont == nil {
			break
		}
	}
	return s, nil
}

func (gs *GenServerS[STATE]) handleInfoRequest(self erl.PID, msg any) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = panicToException(r)
			gs.callback.Terminate(self, err, gs.state)
		}
	}()

	result, err := gs.callback.HandleInfo(self, msg, gs.state)
	gs.state = result.State

	if err != nil {
		err = exitreason.Wrap(err)

		gs.callback.Terminate(self, err, gs.state)
		return err
	}

	if result.Continue != nil {
		state, err := gs.doContinue(self, result.Continue, gs.state)
		gs.state = state
		return err
	}
	return nil
}

func (gs *GenServerS[STATE]) handleCallRequest(self erl.PID, msg CallRequest) (stop bool, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = panicToException(r)
			gs.callback.Terminate(self, err, gs.state)
			stop = true
		}
	}()

	result, err := gs.callback.HandleCall(self, msg.Msg, msg.From, gs.state)

	switch {
	case err != nil:
		if !result.NoReply {
			erl.Send(msg.From.caller, CallReply{Status: OK, Term: result.Msg})
		}
		exit := exitreason.Wrap(err)
		gs.callback.Terminate(self, exit, gs.state)
		return true, exit

	case !result.NoReply:
		erl.Send(msg.From.caller, CallReply{Status: OK, Term: result.Msg}) // genCallerReply{reply: result.Reply})
	default:
		// do nothing
	}
	// update state after we've handled callback errors
	gs.state = result.State

	// invoke HandleContinue callback when we have a ContinueTerm
	if result.Continue != nil {
		state, err := gs.doContinue(self, result.Continue, gs.state)
		if err != nil {
			return true, err
		}
		gs.state = state

	}
	return false, nil
}

func (gs *GenServerS[STATE]) handleCastRequest(self erl.PID, msg CastRequest) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = panicToException(r)
			gs.callback.Terminate(self, err, gs.state)
		}
	}()

	result, err := gs.callback.HandleCast(self, msg.Msg, gs.state)
	if err != nil {
		err = exitreason.Wrap(err)
		gs.callback.Terminate(self, err, gs.state)
		return err
	}
	gs.state = result.State

	// invoke HandleContinue callback when we have a ContinueTerm
	if result.Continue != nil {
		state, err := gs.doContinue(self, result.Continue, gs.state)
		if err != nil {
			return err
		}
		gs.state = state
	}
	return nil
}







