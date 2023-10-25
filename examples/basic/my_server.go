package main

import (
	"github.com/uberbrodt/erl-go/chronos"
	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/genserver"
)

type MyServerState struct {
	count int
}

type MyServer struct{}

var _ genserver.GenServer[MyServerState] = MyServer{}

type (
	PrintCount     struct{}
	IncrementCount struct{}
)

func GetCount(beanCounter erl.Dest) int {
	result, err := genserver.Call(App.Self(), beanCounter, PrintCount{}, chronos.Dur("5s"))
	if err != nil {
		panic(err)
	}
	return result.(int)
}

// CALLBACKS
func (s MyServer) Init(self erl.PID, args any) (genserver.InitResult[MyServerState], error) {
	// send our self a message. We want to get out of [Init] quickly so we don't block up
	// the supervisor. If we need to ABSOlUTELY do something before we recieve our first
	// message, we should implement continue and return a ContinueTerm here.
	erl.Send(self, IncrementCount{})
	return genserver.InitResult[MyServerState]{State: MyServerState{}}, nil
}

func (s MyServer) HandleCall(self erl.PID, request any, from genserver.From, state MyServerState) (genserver.CallResult[MyServerState], error) {
	switch request.(type) {
	case PrintCount:
		return genserver.CallResult[MyServerState]{Msg: state.count, State: state}, nil
	default:
		return genserver.CallResult[MyServerState]{Msg: "idk", State: state}, nil
	}
}

func (s MyServer) HandleCast(self erl.PID, anymsg any, state MyServerState) (genserver.CastResult[MyServerState], error) {
	return genserver.CastResult[MyServerState]{State: state}, nil
}

func (s MyServer) Terminate(self erl.PID, reason error, state MyServerState) {
}

func (s MyServer) HandleContinue(self erl.PID, continuation any, state MyServerState) (MyServerState, error) {
	return state, nil
}

func (s MyServer) HandleInfo(self erl.PID, anymsg any, state MyServerState) (genserver.InfoResult[MyServerState], error) {
	switch anymsg.(type) {
	case IncrementCount:
		state.count = state.count + 1
		erl.SendAfter(self, IncrementCount{}, chronos.Dur("5s"))
	}
	return genserver.InfoResult[MyServerState]{State: state}, nil
}
