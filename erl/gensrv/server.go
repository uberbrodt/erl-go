package gensrv

import (
	"fmt"
	"reflect"
	"time"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"

	"github.com/uberbrodt/erl-go/erl/genserver"
)

// The initial Arg that is received in the Init callback/initially set in StartLink
type config[State any] struct {
	name         erl.Name
	startTimeout time.Duration
	initFun      func(self erl.PID, arg any) (genserver.InitResult[State], error)
	terminateFun func(self erl.PID, reason error, state State)
	castFuns     map[reflect.Type]func(self erl.PID, arg any, state State) (newState State, continu any, err error)
	infoFuns     map[reflect.Type]func(self erl.PID, arg any, state State) (newState State, continu any, err error)
	callFuns     map[reflect.Type]func(self erl.PID, request any, from genserver.From, state State) (genserver.CallResult[State], error)
	continueFuns map[reflect.Type]func(self erl.PID, contTerm any, state State) (State, any, error)
}

// The callback implementation
type CB[State any] struct {
	conf *config[State]
}

// BEGIN CALLBACKS
func (s *CB[State]) Init(self erl.PID, args any) (genserver.InitResult[State], error) {
	var state State

	if s.conf.initFun != nil {
		return s.conf.initFun(self, args)
	}
	return genserver.InitResult[State]{State: state}, nil
}

func (s *CB[State]) HandleCall(self erl.PID, request any, from genserver.From, state State) (genserver.CallResult[State], error) {
	termT := reflect.TypeOf(request)
	for matchTerm, callFun := range s.conf.callFuns {
		if termT == matchTerm {
			return callFun(self, request, from, state)
		}
	}

	return genserver.CallResult[State]{State: state}, exitreason.Exception(fmt.Errorf("no handler for call arg: %+v", request))
}

func (s *CB[State]) HandleCast(self erl.PID, anymsg any, state State) (genserver.CastResult[State], error) {
	termT := reflect.TypeOf(anymsg)
	for matchTerm, castFun := range s.conf.castFuns {
		if termT == matchTerm {
			newState, cont, err := castFun(self, anymsg, state)
			return genserver.CastResult[State]{State: newState, Continue: cont}, err
		}
	}

	return genserver.CastResult[State]{State: state}, exitreason.Exception(fmt.Errorf("no handler for cast arg: %+v", anymsg))
}

func (s *CB[State]) HandleInfo(self erl.PID, anymsg any, state State) (genserver.InfoResult[State], error) {
	termT := reflect.TypeOf(anymsg)
	for matchTerm, infoFun := range s.conf.infoFuns {
		if termT == matchTerm {
			newState, cont, err := infoFun(self, anymsg, state)
			return genserver.InfoResult[State]{State: newState, Continue: cont}, err
		}
	}

	return genserver.InfoResult[State]{State: state}, exitreason.Exception(fmt.Errorf("no handler for info arg: %+v", anymsg))
}

func (s *CB[State]) HandleContinue(self erl.PID, continuation any, state State) (State, any, error) {
	termT := reflect.TypeOf(continuation)
	for matchTerm, contFun := range s.conf.continueFuns {
		if termT == matchTerm {
			newState, cont, err := contFun(self, continuation, state)
			return newState, cont, err
		}
	}

	return state, nil, exitreason.Exception(fmt.Errorf("no handler for continuation arg: %+v", continuation))
}

func (s *CB[State]) Terminate(self erl.PID, reason error, state State) {
	if s.conf.terminateFun != nil {
		s.conf.terminateFun(self, reason, state)
	}
}
