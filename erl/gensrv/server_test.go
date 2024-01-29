package gensrv

import (
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/genserver"
)

type testState struct {
	sum int
}

type (
	add                int
	sub                int
	getSum             struct{}
	notifyTestReceiver struct{}
)

var initFn = RegisterInit[testState](func(self erl.PID, arg any) (genserver.InitResult[testState], error) {
	startSum := arg.(int)

	return genserver.InitResult[testState]{State: testState{sum: startSum}}, nil
})

var addFn = func(self erl.PID, a any, state testState) (newState testState, continu any, err error) {
	arg := a.(add)
	state.sum = state.sum + int(arg)
	return state, nil, nil
}

var subFn = func(self erl.PID, a any, state testState) (newState testState, continu any, err error) {
	arg := a.(sub)
	state.sum = state.sum - int(arg)
	return state, nil, nil
}

var addCast = RegisterCast[testState](add(0), addFn)

var addCastNotify = RegisterCast[testState](add(0), func(self erl.PID, a any, state testState) (newState testState, continu any, err error) {
	newState, _, err = addFn(self, a, state)
	return newState, notifyTestReceiver{}, err
})

var subCast = RegisterCast[testState](sub(0), subFn)

var subInfo = RegisterInfo[testState](sub(0), subFn)

var subCastNotify = RegisterCast[testState](sub(0), func(self erl.PID, a any, state testState) (newState testState, continu any, err error) {
	arg := a.(sub)
	state.sum = state.sum - int(arg)
	return state, notifyTestReceiver{}, nil
})

var sumCall = RegisterCall[testState](getSum{},
	func(self erl.PID, request any, from genserver.From, state testState) (genserver.CallResult[testState], error) {
		return genserver.CallResult[testState]{Msg: state.sum, State: state}, nil
	})

func TestServer_MatchesCast(t *testing.T) {
	testPID, _ := erl.NewTestReceiver(t)

	pid, err := StartLink(testPID, nil, addCast, sumCall, subCast)

	assert.NilError(t, err)

	genserver.Cast(pid, add(12))
	genserver.Cast(pid, add(3))
	result, err := genserver.Call(testPID, pid, getSum{}, 5*time.Second)

	assert.NilError(t, err)
	assert.Equal(t, result, 15)
	genserver.Cast(pid, sub(4))

	result, err = genserver.Call(testPID, pid, getSum{}, 5*time.Second)
	assert.NilError(t, err)

	assert.Equal(t, result, 11)
}

func TestServer_InitArg(t *testing.T) {
	testPID, tr := erl.NewTestReceiver(t)

	notifyCont := RegisterContinue[testState](notifyTestReceiver{}, func(self erl.PID, cont any, state testState) (newState testState, continu any, err error) {
		erl.Send(testPID, state.sum)
		return state, nil, nil
	})

	pid, err := StartLink(testPID, 10, initFn, addCastNotify, sumCall, subCastNotify, notifyCont)

	assert.NilError(t, err)

	genserver.Cast(pid, add(12))
	result, err := genserver.Call(testPID, pid, getSum{}, 5*time.Second)
	assert.NilError(t, err)
	assert.Equal(t, result, 22)

	tr.Loop(func(anymsg any) bool {
		switch msg := anymsg.(type) {
		case int:
			assert.Equal(t, msg, 22)
			return true
		default:
			return false

		}
	})
}

func TestServer_RegisterInfo(t *testing.T) {
	testPID, _ := erl.NewTestReceiver(t)

	pid, err := StartLink(testPID, 10, initFn, addCast, sumCall, subInfo)

	assert.NilError(t, err)

	erl.Send(pid, sub(3))
	result, err := genserver.Call(testPID, pid, getSum{}, 5*time.Second)
	assert.NilError(t, err)
	assert.Equal(t, result, 7)
}
