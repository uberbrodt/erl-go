package recurringtask

import (
	"errors"
	"time"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
	"github.com/uberbrodt/erl-go/erl/genserver"
)

type rtConfig[STATE any, ARGS any] struct {
	// Executed on each loopTimeout. return error to stop the function
	taskFun func(self erl.PID, state STATE) (STATE, error)
	// Setup the initialState. This should be executed in GenServer.Init
	initFun func(self erl.PID, args ARGS) (STATE, error)
	// How often to execute the taskFun
	loopTimeout time.Duration
	taskArgs    ARGS
}

type rtSrv[STATE any, ARGS any] struct{}

type rtState[STATE any, ARGS any] struct {
	taskState STATE
	conf      rtConfig[STATE, ARGS]
}

type doTask struct{}

// CALLBACKS
func (s *rtSrv[S, A]) Init(self erl.PID, args any) (genserver.InitResult[rtState[S, A]], error) {
	conf, ok := args.(rtConfig[S, A])
	if !ok {
		return genserver.InitResult[rtState[S, A]]{}, exitreason.Exception(errors.New("rtSrv Init arg must be a WatchdogConfig{}"))
	}

	state, err := conf.initFun(self, conf.taskArgs)
	if err == nil {
		erl.Send(self, doTask{})
	}

	return genserver.InitResult[rtState[S, A]]{State: rtState[S, A]{taskState: state, conf: conf}}, err
}

func (s *rtSrv[S, A]) HandleCall(self erl.PID, request any, from genserver.From, state rtState[S, A]) (genserver.CallResult[rtState[S, A]], error) {
	return genserver.CallResult[rtState[S, A]]{Msg: "unsupported", State: state}, nil
}

func (s *rtSrv[S, A]) HandleCast(self erl.PID, anymsg any, state rtState[S, A]) (genserver.CastResult[rtState[S, A]], error) {
	return genserver.CastResult[rtState[S, A]]{State: state}, nil
}

func (s *rtSrv[S, A]) Terminate(self erl.PID, reason error, state rtState[S, A]) {}

func (s *rtSrv[S, A]) HandleContinue(self erl.PID, continuation any, state rtState[S, A]) (rtState[S, A], error) {
	return state, nil
}

func (s *rtSrv[S, A]) HandleInfo(self erl.PID, anymsg any, state rtState[S, A]) (genserver.InfoResult[rtState[S, A]], error) {
	switch msg := anymsg.(type) {
	case doTask:
		erl.DebugPrintf("%v running task", self)
		taskState, err := state.conf.taskFun(self, state.taskState)
		state.taskState = taskState
		erl.SendAfter(self, doTask{}, state.conf.loopTimeout)

		return genserver.InfoResult[rtState[S, A]]{State: state}, err
	default:
		erl.DebugPrintf("got unhandled message: %+v", msg)
		return genserver.InfoResult[rtState[S, A]]{State: state}, nil
	}
}
