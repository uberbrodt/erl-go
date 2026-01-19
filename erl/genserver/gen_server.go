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

type (
	InitResult[STATE any] struct {
		State    STATE
		Continue any
	}
	CallResult[STATE any] struct {
		// if true, then no reply will be sent to the caller. The genserver should reply with [genserver.Reply]
		// at a later time, otherwise the caller will time out.
		NoReply bool
		// The reply that will be sent to the caller
		Msg any
		// The updated state of the GenServer
		State STATE
		// if not nil, will call [HandleContinue] immmediately after [HandleCall] returns with this as the [continuation]
		Continue any
	}
	CastResult[STATE any] struct {
		State    STATE
		Continue any
	}
	InfoResult[STATE any] struct {
		State    STATE
		Continue any
	}
)

type GenServer[STATE any] interface {
	Init(self erl.PID, args any) (InitResult[STATE], error)
	HandleCall(self erl.PID, request any, from From, state STATE) (CallResult[STATE], error)
	HandleCast(self erl.PID, request any, state STATE) (CastResult[STATE], error)
	HandleInfo(self erl.PID, msg any, state STATE) (InfoResult[STATE], error)
	// Handles Continuation terms from other callbacks. Set the [continueTerm] to re-enter the [HandleContinue]
	// callback with a new state
	HandleContinue(self erl.PID, continuation any, state STATE) (newState STATE, continueTerm any, err error)
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

func (gs *GenServerS[STATE]) handleInit(self erl.PID, msg any) (result InitResult[STATE], err error) {
	defer func() {
		if r := recover(); r != nil {
			e, ok := r.(error)
			if !ok {
				err = exitreason.Exception(fmt.Errorf("panic in init: %v", r))
			} else {
				if !exitreason.IsException(e) {
					err = exitreason.Exception(e)
				} else {
					err = e
				}
			}

		}
	}()
	result, err = gs.callback.Init(self, gs.args)
	return result, err
}

func (gs *GenServerS[STATE]) doContinue(self erl.PID, inCont any, inState STATE) (STATE, error) {
	s := inState
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

func (gs *GenServerS[STATE]) handleInfoRequest(self erl.PID, msg any) error {
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

func (gs *GenServerS[STATE]) handleCallRequest(self erl.PID, msg CallRequest) (bool, error) {
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

func (gs *GenServerS[STATE]) handleCastRequest(self erl.PID, msg CastRequest) error {
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
