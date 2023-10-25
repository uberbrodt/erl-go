package main

import (
	"log"

	"github.com/uberbrodt/erl-go/chronos"
	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
	"github.com/uberbrodt/erl-go/erl/genserver"
	"github.com/uberbrodt/erl-go/erl/port"
)

type MyServerState struct {
	count   int
	portPID erl.PID
}

type MyServer struct{}

var _ genserver.GenServer[MyServerState] = MyServer{}

type (
	PrintCount     struct{}
	IncrementCount struct{}
)

type (
	startPort struct{}
	closePort struct{}
)

// CALLBACKS
func (s MyServer) Init(self erl.PID, args any) (genserver.InitResult[MyServerState], error) {
	return genserver.InitResult[MyServerState]{State: MyServerState{}, Continue: startPort{}}, nil
}

func (s MyServer) HandleCall(self erl.PID, request any, from genserver.From, state MyServerState) (genserver.CallResult[MyServerState], error) {
	return genserver.CallResult[MyServerState]{Msg: "idk", State: state}, nil
}

func (s MyServer) HandleCast(self erl.PID, anymsg any, state MyServerState) (genserver.CastResult[MyServerState], error) {
	return genserver.CastResult[MyServerState]{State: state}, nil
}

func (s MyServer) Terminate(self erl.PID, reason error, state MyServerState) {
}

func (s MyServer) HandleContinue(self erl.PID, continuation any, state MyServerState) (MyServerState, error) {
	switch continuation.(type) {
	case startPort:
		log.Printf("starting port")
		state.portPID = port.Open(self, "./testport.sh")
		erl.SendAfter(self, closePort{}, chronos.Dur("10s"))
	}
	return state, nil
}

func (s MyServer) HandleInfo(self erl.PID, anymsg any, state MyServerState) (genserver.InfoResult[MyServerState], error) {
	switch msg := anymsg.(type) {
	case closePort:
		erl.Unlink(self, state.portPID)
		port.Close(self, state.portPID)
		return genserver.InfoResult[MyServerState]{State: state}, exitreason.Normal
	default:
		log.Printf("MyServer got unhandled message: %+v", msg)
	}
	return genserver.InfoResult[MyServerState]{State: state}, nil
}
