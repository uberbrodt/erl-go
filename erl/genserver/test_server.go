package genserver

import "github.com/uberbrodt/erl-go/erl"

type TestMsg[STATE any] struct {
	// probe can be used to inject functionality, reply back in cast requests, etc.
	Probe         func(self erl.PID, arg any, state STATE) (cont any, newState STATE, err error)
	CallProbe     func(self erl.PID, arg any, from From, state STATE) (call CallResult[STATE], err error)
	ContinueProbe func(self erl.PID, state STATE) (newState STATE, err error)
	Arg           any
}

// func(tm TestMsg[STATE]) TestMsg[STATE]

func SetProbe[STATE any](probe func(self erl.PID, arg any, state STATE) (cont any, newState STATE, err error)) func(tm TestMsg[STATE]) TestMsg[STATE] {
	return func(tm TestMsg[STATE]) TestMsg[STATE] {
		tm.Probe = probe
		return tm
	}
}

func SetCallProbe[STATE any](probe func(self erl.PID, arg any, from From, state STATE) (call CallResult[STATE], err error)) func(tm TestMsg[STATE]) TestMsg[STATE] {
	return func(tm TestMsg[STATE]) TestMsg[STATE] {
		tm.CallProbe = probe
		return tm
	}
}

func SetContinueProbe[STATE any](probe func(self erl.PID, state STATE) (newState STATE, err error)) func(tm TestMsg[STATE]) TestMsg[STATE] {
	return func(tm TestMsg[STATE]) TestMsg[STATE] {
		tm.ContinueProbe = probe
		return tm
	}
}

func SetArg[STATE any](arg any) func(tm TestMsg[STATE]) TestMsg[STATE] {
	return func(tm TestMsg[STATE]) TestMsg[STATE] {
		tm.Arg = arg
		return tm
	}
}

func NewTestMsg[STATE any](opts ...func(tm TestMsg[STATE]) TestMsg[STATE]) TestMsg[STATE] {
	tm := TestMsg[STATE]{}
	for _, opt := range opts {
		tm = opt(tm)
	}
	return tm
}

type TestServer[STATE any] struct {
	TermProbe    func(self erl.PID, reason error, state STATE)
	InitProbe    func(self erl.PID, args any) (STATE, any, error)
	InitialState STATE
	TestReceiver erl.PID
}

type TestNotifInit[STATE any] struct {
	Self  erl.PID
	State STATE
	Args  any
	Error error
}

type TestNotifCall[STATE any] struct {
	Self    erl.PID
	Result  CallResult[STATE]
	Request TestMsg[STATE]
	Error   error
}

type TestNotifInfo[STATE any] struct {
	Self    erl.PID
	State   STATE
	Request TestMsg[STATE]
	Error   error
}

type TestNotifCast[STATE any] struct {
	Self    erl.PID
	State   STATE
	Request TestMsg[STATE]
	Error   error
}

type TestNotifContinue[STATE any] struct {
	Self    erl.PID
	State   STATE
	Request TestMsg[STATE]
	Error   error
}

type TestNotifTerminate[STATE any] struct {
	Self   erl.PID
	State  STATE
	Reason error
}

func SetInitProbe[STATE any](probe func(self erl.PID, args any) (STATE, any, error)) func(ts TestServer[STATE]) TestServer[STATE] {
	return func(ts TestServer[STATE]) TestServer[STATE] {
		ts.InitProbe = probe
		return ts
	}
}

func SetTermProbe[STATE any](probe func(self erl.PID, reason error, state STATE)) func(ts TestServer[STATE]) TestServer[STATE] {
	return func(ts TestServer[STATE]) TestServer[STATE] {
		ts.TermProbe = probe
		return ts
	}
}

func SetInitialState[STATE any](state STATE) func(ts TestServer[STATE]) TestServer[STATE] {
	return func(ts TestServer[STATE]) TestServer[STATE] {
		ts.InitialState = state
		return ts
	}
}

func SetTestReceiver[STATE any](tr erl.PID) func(ts TestServer[STATE]) TestServer[STATE] {
	return func(ts TestServer[STATE]) TestServer[STATE] {
		ts.TestReceiver = tr
		return ts
	}
}

func NewTestServer[STATE any](opts ...func(ts TestServer[STATE]) TestServer[STATE]) TestServer[STATE] {
	ts := TestServer[STATE]{}
	for _, opt := range opts {
		ts = opt(ts)
	}
	return ts
}

func (ts TestServer[STATE]) Init(self erl.PID, args any) (InitResult[STATE], error) {
	state := ts.InitialState
	var cont any
	var err error
	initResult := InitResult[STATE]{State: ts.InitialState}
	if ts.InitProbe != nil {
		state, cont, err = ts.InitProbe(self, args)
		initResult = InitResult[STATE]{State: state, Continue: cont}
	}

	if !ts.TestReceiver.IsNil() {
		erl.Link(ts.TestReceiver, self)
		erl.Send(ts.TestReceiver, TestNotifInit[STATE]{Self: self, State: state, Args: args, Error: err})
	}

	return initResult, err
}

func (ts TestServer[STATE]) HandleCall(self erl.PID, request any, from From, state STATE) (CallResult[STATE], error) {
	req := request.(TestMsg[STATE])
	callResult := CallResult[STATE]{Msg: request, State: state}
	var err error

	if req.CallProbe != nil {
		callResult, err = req.CallProbe(self, request, from, state)
	}

	if !ts.TestReceiver.IsNil() {
		erl.Send(ts.TestReceiver, TestNotifCall[STATE]{Self: self, Request: req, Result: callResult})
	}
	return callResult, err
}

func (ts TestServer[STATE]) HandleCast(self erl.PID, request any, state STATE) (CastResult[STATE], error) {
	req := request.(TestMsg[STATE])
	var cont any
	var err error
	newState := state
	castResult := CastResult[STATE]{State: newState}

	if req.Probe != nil {

		cont, newState, err = req.Probe(self, request, state)
		castResult = CastResult[STATE]{State: newState, Continue: cont}
	}

	if !ts.TestReceiver.IsNil() {
		erl.Send(ts.TestReceiver, TestNotifCast[STATE]{Self: self, State: newState, Request: req, Error: err})
	}

	return castResult, err
}

func (ts TestServer[STATE]) HandleInfo(self erl.PID, request any, state STATE) (InfoResult[STATE], error) {
	req := request.(TestMsg[STATE])
	var cont any
	var err error
	newState := state
	infoResult := InfoResult[STATE]{State: state}

	if req.Probe != nil {
		cont, newState, err = req.Probe(self, request, state)
		infoResult = InfoResult[STATE]{State: newState, Continue: cont}
	}

	if !ts.TestReceiver.IsNil() {
		erl.Send(ts.TestReceiver, TestNotifInfo[STATE]{Self: self, State: newState, Request: req, Error: err})
	}
	return infoResult, err
}

func (ts TestServer[STATE]) HandleContinue(self erl.PID, continuation any, state STATE) (STATE, error) {
	req := continuation.(TestMsg[STATE])
	newState := state
	var err error

	if req.ContinueProbe != nil {
		newState, err = req.ContinueProbe(self, state)
	}

	if !ts.TestReceiver.IsNil() {
		erl.Send(ts.TestReceiver, TestNotifContinue[STATE]{Self: self, State: newState, Request: req, Error: err})
	}
	return state, nil
}

func (ts TestServer[STATE]) Terminate(self erl.PID, reason error, state STATE) {
	if ts.TermProbe != nil {
		ts.TermProbe(self, reason, state)
	}

	if !ts.TestReceiver.IsNil() {
		erl.Send(ts.TestReceiver, TestNotifTerminate[STATE]{Self: self, State: state, Reason: reason})
	}
}
