package erl_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/erltest"
	"github.com/uberbrodt/erl-go/erl/exitreason"
	"github.com/uberbrodt/erl-go/erl/genserver"
	"github.com/uberbrodt/erl-go/erl/gensrv"
	"github.com/uberbrodt/erl-go/erl/supervisor"
)

// when a producer or consumer receives this message, it
// replies back with the [count] incremented by 1
type Inc struct {
	workerID string
	worker   erl.PID
	producer erl.PID
	count    int
}

type GetStopSignal struct{}

type ReturnStopSignal struct {
	stopSignal <-chan struct{}
}

// The initial Arg that is received in the Init callback/initially set in StartLink
type workerConfig struct {
	workerID string
	producer erl.PID
	// the count to stop at
	stop int
	t    *testing.T
}

// The server state
type workerState struct {
	workerID string
	producer erl.PID
	// the last seen value.
	count      int
	stop       int
	stopSignal chan struct{}
	t          *testing.T
}

// Adds a named process to a Supervisor. Many servers are autonomous and do not need a Name, so consider refactoring if that's the case
func WorkerChildSpec(config workerConfig, opts ...supervisor.ChildSpecOpt) supervisor.ChildSpec {
	return supervisor.NewChildSpec(config.workerID,
		func(self erl.PID) (erl.PID, error) {
			return WorkerStartLink(self, config)
		}, opts...,
	)
}

func WorkerStartLink(self erl.PID, conf workerConfig) (erl.PID, error) {
	return gensrv.StartLink[workerState](self, conf,
		gensrv.RegisterInit(workerInit),
		gensrv.RegisterInfo(Inc{}, workerHandleInc),
		gensrv.RegisterCall(GetStopSignal{}, workerReturnStopSignal),
	)
}

// Initialization function. Called when a process is started. Supervisors will block until this function returns.
func workerInit(self erl.PID, args any) (workerState, any, error) {
	conf, ok := args.(workerConfig)

	if !ok {
		return workerState{}, nil, exitreason.Exception(errors.New("Init arg must be a workerConfig{}"))
	}
	erl.Send(self, Inc{count: 0, producer: conf.producer, worker: self, workerID: conf.workerID})
	return workerState{
		producer:   conf.producer,
		stop:       conf.stop,
		stopSignal: make(chan struct{}),
		workerID:   conf.workerID,
		t:          conf.t,
	}, nil, nil
}

func workerHandleInc(self erl.PID, msg Inc, state workerState) (workerState, any, error) {
	msg.count = msg.count + 1

	if msg.count > state.count+1 {
		// slog.Error(,
		// 	"msg_count", msg.count, "expected_state", state.count+1, "worker_id", state.workerID)
		if state.t != nil {
			state.t.Errorf("the msg count is greater than what we expected in the worker state: msg_count: %d, expected_state: %d, worker_id: %s",
				msg.count, state.count+1, msg.workerID)
		}
		return state, nil, exitreason.Normal
	}

	if msg.count < state.count {
		// slog.Error("the msg count is less than what we expected in the worker state",
		// 	"msg_count", msg.count, "expected_state", state.count, "worker_id", state.workerID)
		if state.t != nil {
			state.t.Errorf("the msg count is less than what we expected in the worker state: msg_count: %d, expected_state: %d, worker_id: %s",
				msg.count, state.count+1, msg.workerID)
		}
		return state, nil, exitreason.Normal
	}
	state.count = msg.count

	if state.t != nil {
		state.t.Logf("[%d] worker %s replying: %d ", time.Now().UnixMilli(), msg.workerID, msg.count)
	}
	erl.Send(msg.producer, msg)

	if msg.count >= state.stop {
		close(state.stopSignal)
		return state, nil, exitreason.Normal
	}

	return state, nil, nil
}

func workerReturnStopSignal(self erl.PID, msg GetStopSignal, from genserver.From, state workerState) (result genserver.CallResult[workerState], err error) {
	return genserver.CallResult[workerState]{State: state, Msg: ReturnStopSignal{stopSignal: state.stopSignal}}, nil
}

// The initial Arg that is received in the Init callback/initially set in StartLink
type producerConfig struct {
	stop int
	t    *testing.T
}

// The server state
type producerState struct {
	workerCounts map[string]int
	// if all worker counts equal this, the producer will shutdown
	stop       int
	stopSignal chan struct{}
	t          *testing.T
}

// Adds a named process to a Supervisor. Many servers are autonomous and do not need a Name, so consider refactoring if that's the case
func producerChildSpec(name erl.Name, config producerConfig, opts ...supervisor.ChildSpecOpt) supervisor.ChildSpec {
	return supervisor.NewChildSpec(string(name),
		func(self erl.PID) (erl.PID, error) {
			return producerStartLink(self, name, config)
		}, opts...,
	)
}

func producerStartLink(self erl.PID, name erl.Name, conf producerConfig) (erl.PID, error) {
	return gensrv.StartLink[producerState](self, conf,
		gensrv.SetName[producerState](name),
		gensrv.RegisterInit(producerInit),
		gensrv.RegisterInfo(Inc{}, producerHandleInc),
	)
}

// Initialization function. Called when a process is started. Supervisors will block until this function returns.
func producerInit(self erl.PID, args any) (producerState, any, error) {
	conf, ok := args.(producerConfig)

	if !ok {
		return producerState{}, nil, exitreason.Exception(errors.New("Init arg must be a producerConfig{}"))
	}
	return producerState{
		stop:         conf.stop,
		stopSignal:   make(chan struct{}),
		workerCounts: make(map[string]int),
		t:            conf.t,
	}, nil, nil
}

func producerHandleInc(self erl.PID, msg Inc, state producerState) (producerState, any, error) {
	if _, ok := state.workerCounts[msg.workerID]; !ok {
		state.workerCounts[msg.workerID] = msg.count
	}

	if msg.count > state.workerCounts[msg.workerID]+1 {
		// slog.Error("the msg count is greater than what we expected in the producer state",
		// 	"msg_count", msg.count, "expected_state", state.workerCounts[msg.workerID]+1, "worker_id", msg.workerID)
		if state.t != nil {
			state.t.Errorf(
				"the msg count is greater than what we expected in the producer state: msg_count: %d, expected_state: %d, worker_id: %s",
				msg.count, state.workerCounts[msg.workerID]+1, msg.workerID)
		}
		return state, nil, exitreason.Normal
	}

	if msg.count < state.workerCounts[msg.workerID] {
		if state.t != nil {
			state.t.Errorf(
				"the msg count is less than what we expected in the producer state: msg_count: %d, expected_state: %d, worker_id: %s",
				msg.count, state.workerCounts[msg.workerID], msg.workerID)
		}
		return state, nil, exitreason.Normal
	}
	state.workerCounts[msg.workerID] = msg.count

	// check to see if all workers are done incrementing
	done := true
	for _, cnt := range state.workerCounts {
		if cnt < state.stop {
			done = false
		}
	}
	if done {
		close(state.stopSignal)
		if state.t != nil {
			state.t.Log("producer done")
		}
		return state, nil, exitreason.Normal
	}

	if state.t != nil {
		state.t.Logf("[%d] producer replying to worker %s: %d", time.Now().UnixMilli(), msg.workerID, msg.count)
	}
	erl.Send(msg.worker, msg)

	return state, nil, nil
}

func producerReturnStopSignal(self erl.PID,
	msg GetStopSignal,
	from genserver.From,
	state producerState,
) (result genserver.CallResult[producerState], err error) {
	return genserver.CallResult[producerState]{State: state, Msg: ReturnStopSignal{stopSignal: state.stopSignal}}, nil
}

func ProducerChildSpec(config producerConfig, opts ...supervisor.ChildSpecOpt) supervisor.ChildSpec {
	return supervisor.NewChildSpec("fan-out-supervisor",
		func(self erl.PID) (erl.PID, error) {
			return ProducerStartLink(self, config)
		}, opts...,
	)
}

func ProducerStartLink(self erl.PID, conf producerConfig) (erl.PID, error) {
	return gensrv.StartLink(self, conf,
		gensrv.RegisterInit(producerInit),
		gensrv.RegisterInfo(Inc{}, producerHandleInc),
		gensrv.RegisterCall(GetStopSignal{}, producerReturnStopSignal),
	)
}

func TestExt_WorkerFanOut(t *testing.T) {
	testPID, _ := erltest.NewReceiver(t, erltest.WaitTimeout(3*time.Minute))
	stop := 500
	workerCnt := 16

	producer, err := ProducerStartLink(testPID, producerConfig{stop: stop, t: t})
	assert.NilError(t, err)

	result, err := genserver.Call(testPID, producer, GetStopSignal{}, 10*time.Second)

	assert.NilError(t, err)

	stopsigreply := result.(ReturnStopSignal)
	producerSignal := stopsigreply.stopSignal

	workerSignals := make([]<-chan struct{}, 0)

	for i := range workerCnt {
		worker, err := WorkerStartLink(testPID, workerConfig{
			producer: producer,
			stop:     stop,
			workerID: fmt.Sprintf("worker-%d", i),
			t:        t,
		})

		assert.NilError(t, err)

		result, err := genserver.Call(testPID, worker, GetStopSignal{}, 10*time.Second)

		assert.NilError(t, err)

		x := result.(ReturnStopSignal)
		workerSignals = append(workerSignals, x.stopSignal)
	}

	for _, s := range workerSignals {
		<-s
	}
	<-producerSignal
}

func BenchmarkErlGoProducerMultipleConsumers(b *testing.B) {
	for range b.N {

		// testPID, _ := erltest.NewReceiver(t, erltest.WaitTimeout(3*time.Minute))
		stop := 1000
		workerCnt := 8

		producer, err := ProducerStartLink(erl.RootPID(), producerConfig{stop: stop})
		if err != nil {
			b.Fatal(err)
		}

		result, err := genserver.Call(erl.RootPID(), producer, GetStopSignal{}, 10*time.Second)

		stopsigreply := result.(ReturnStopSignal)
		producerSignal := stopsigreply.stopSignal

		workerSignals := make([]<-chan struct{}, 0)

		for i := range workerCnt {
			worker, err := WorkerStartLink(erl.RootPID(), workerConfig{
				producer: producer,
				stop:     stop,
				workerID: fmt.Sprintf("worker-%d", i),
			})
			if err != nil {
				b.Fatal(err)
			}

			result, err := genserver.Call(erl.RootPID(), worker, GetStopSignal{}, 10*time.Second)
			if err != nil {
				b.Fatal(err)
			}

			x := result.(ReturnStopSignal)
			workerSignals = append(workerSignals, x.stopSignal)
		}

		for _, s := range workerSignals {
			<-s
		}
		<-producerSignal

	}
}
