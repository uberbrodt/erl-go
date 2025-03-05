package genserver_test

import (
	"testing"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/x/erltest/testserver"
)

type DownNotification struct {
	msg erl.DownMsg
}

func parentGetsDownMsg(t *testing.T, parentPID erl.PID) func(self erl.PID, anymsg any, state testserver.TestServer) (newState testserver.TestServer, continu any, err error) {
	return func(self erl.PID, anymsg any, state testserver.TestServer) (testserver.TestServer, any, error) {
		msg := anymsg.(erl.DownMsg)
		t.Log("Relaying down msg to parent PID")
		erl.Send(parentPID, DownNotification{msg: msg})
		return state, nil, nil
	}
}
