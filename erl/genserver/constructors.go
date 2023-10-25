package genserver

import (
	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
)

// When you just need core HandleInfo, HandleCall, HandleCast, behaviour. Will implement
// a sane default for Terminate
type CoreGenServer[STATE any] interface {
	Init(self erl.PID, args any) InitResult[STATE]
	HandleCall(self erl.PID, request any, from From, state STATE) CallResult[STATE]
	HandleInfo(self erl.PID, msg any, state STATE) (InfoResult[STATE], error)
}

type CoreGenServerS[STATE any] struct {
	cgs CoreGenServer[STATE]
}

func (s CoreGenServerS[STATE]) Init(self erl.PID, args any) InitResult[STATE] {
	return s.cgs.Init(self, args)
}

func (s CoreGenServerS[STATE]) HandleCall(self erl.PID, request any, from From, state STATE) CallResult[STATE] {
	return s.cgs.HandleCall(self, request, from, state)
}

func (s CoreGenServerS[STATE]) HandleInfo(self erl.PID, request any, state STATE) (InfoResult[STATE], error) {
	return s.cgs.HandleInfo(self, request, state)
}

func (s CoreGenServerS[STATE]) HandleContinue(self erl.PID, continuation any, state STATE) (STATE, error) {
	erl.Logger.Printf("You used Continue with arg %+v but did not implement it. Stopping", continuation)
	return state, nil
}

func (s CoreGenServerS[STATE]) HandleTerminate(self erl.PID, reason *exitreason.S, state STATE) {
}

// Builds a GenServer implementation that wraps a [CoreGenServer] and implements
// some non-critical [GenServer] methods.
func NewCoreGenServer[STATE any](cgs CoreGenServer[STATE]) CoreGenServerS[STATE] {
	return CoreGenServerS[STATE]{cgs: cgs}
}
