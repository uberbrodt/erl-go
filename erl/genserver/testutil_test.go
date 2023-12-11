package genserver

import (
	"errors"
	"testing"
	"time"

	"github.com/uberbrodt/erl-go/chronos"
	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
	"github.com/uberbrodt/erl-go/erl/timeout"
)

type TestReceiver struct {
	c chan any
	t *testing.T
}

func (th *TestReceiver) Receive(self erl.PID, inbox <-chan any) error {
	for {
		select {
		case msg := <-inbox:
			th.c <- msg
		case <-time.After(chronos.Dur("30s")):
			th.t.Log("test timeout")
			close(th.c)

			return exitreason.Timeout
		}
	}
}

func NewTestReceiver(t *testing.T) (erl.PID, chan any) {
	c := make(chan any, 50)
	tr := &TestReceiver{c: c, t: t}
	pid := erl.Spawn(tr)

	erl.ProcessFlag(pid, erl.TrapExit, true)
	t.Cleanup(func() {
		erl.Exit(erl.RootPID(), pid, exitreason.SupervisorShutdown)
	})
	return pid, c
}

func startTestGS(self erl.PID, t *testing.T, cb TestGS, args any, opts ...StartOpt) (erl.PID, error) {
	pid, err := StartLink[TestGS](self, cb, args, opts...)
	if err != nil {
		t.Cleanup(func() {
			erl.Exit(erl.RootPID(), pid, exitreason.Kill)
		})
	}
	return pid, err
}

type TestGS struct {
	Count          int
	from           From
	terminateProbe func(self erl.PID, arg error, state TestGS)
}

type TestGSArgs struct {
	initProbe func(self erl.PID, args any) (state TestGS, cont any, err error)
	count     int
}

var _ GenServer[TestGS] = TestGS{}

type taggedRequest struct {
	tag   string
	value any
	// probe can be used to inject functionality, reply back in cast requests, etc.
	probe         func(self erl.PID, state TestGS) (newState TestGS)
	callProbe     func(self erl.PID, arg any, from From, state TestGS) (reply any, newState TestGS)
	continueProbe func(self erl.PID, state TestGS) (newState TestGS, continuation any, err error)
	err           error
	cont          bool
}

type continueArg struct {
	reply From
	wait  time.Duration
	arg   any
}

func (gs TestGS) Init(self erl.PID, args any) (InitResult[TestGS], error) {
	argsT := args.(TestGSArgs)

	if argsT.initProbe != nil {
		state, cont, err := argsT.initProbe(self, args)
		return InitResult[TestGS]{State: state, Continue: cont}, err
	} else {
		return InitResult[TestGS]{State: TestGS{Count: argsT.count}}, nil
	}
}

func getTestGSState(gensrv erl.PID) (TestGS, error) {
	any, err := Call(erl.RootPID(), gensrv, taggedRequest{tag: "get_state"}, timeout.Default)
	if err != nil {
		return TestGS{}, err
	}

	state, ok := any.(TestGS)
	if !ok {
		return TestGS{}, errors.New("returned reply was not the server state")
	}
	return state, nil
}

func (gs TestGS) HandleCall(self erl.PID, request any, from From, state TestGS) (CallResult[TestGS], error) {
	req := request.(taggedRequest)

	reply, state := req.callProbe(self, req.value, from, state)
	noreply := reply == nil

	if req.err != nil {
		return CallResult[TestGS]{State: state, NoReply: noreply, Msg: reply}, req.err
	}

	if req.cont {
		return CallResult[TestGS]{State: state, NoReply: noreply, Msg: reply, Continue: request}, nil
	}

	return CallResult[TestGS]{NoReply: noreply, Msg: reply, State: state}, nil
}

func (gs TestGS) HandleCast(self erl.PID, request any, state TestGS) (CastResult[TestGS], error) {
	req := request.(taggedRequest)

	if req.err != nil {
		return CastResult[TestGS]{}, req.err
	}

	if req.probe != nil {
		state = req.probe(self, state)
	}

	if req.cont {
		return CastResult[TestGS]{State: state, Continue: request}, nil
	}

	return CastResult[TestGS]{State: state}, nil
}

func (ts TestGS) HandleInfo(self erl.PID, request any, state TestGS) (InfoResult[TestGS], error) {
	erl.Logger.Printf("%v HandleInfo got msg: %v", self, request)
	req := request.(taggedRequest)

	if req.err != nil {
		return InfoResult[TestGS]{}, req.err
	}

	if req.probe != nil {
		state = req.probe(self, state)
	}

	if req.cont {
		return InfoResult[TestGS]{State: state, Continue: request}, nil
	}

	return InfoResult[TestGS]{State: state}, nil
}

func (gs TestGS) Terminate(self erl.PID, arg error, state TestGS) {
	if gs.terminateProbe != nil {
		gs.terminateProbe(self, exitreason.Wrap(arg), state)
	}
}

func (gs TestGS) HandleContinue(self erl.PID, continuation any, state TestGS) (TestGS, any, error) {
	req := continuation.(taggedRequest)
	if req.continueProbe != nil {
		return req.continueProbe(self, state)
	}
	return state, nil, nil
}
