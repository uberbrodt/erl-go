/*
 * This package contains a server that will help you block execution while waiting on a process to exit.
 */
package exitwaiter

import (
	"errors"
	"log/slog"
	"sync"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
	"github.com/uberbrodt/erl-go/erl/gensrv"
)

// The initial Arg that is received in the Init callback/initially set in StartLink
type weConfig struct {
	wg       *sync.WaitGroup
	waitinOn erl.PID
}

// The server state
type westate struct {
	conf weConfig
	ref  erl.Ref
}

// a testing.T
type TestT interface {
	FailNow()
	Logf(format string, args ...any)
}

// Starts a new ExitWaiter. The returned channel will be closed when the exit
// signal is received from [waitingOn]
func New(t TestT, self erl.PID, waitingOn erl.PID, wg *sync.WaitGroup) (erl.PID, error) {
	pid, err := Start(self, weConfig{wg: wg, waitinOn: waitingOn})
	if err != nil {
		t.Logf("could not start exitWaiter: %v", err)
		t.FailNow()
		return pid, err
	}
	return pid, err
}

func Start(self erl.PID, conf weConfig) (erl.PID, error) {
	return gensrv.Start(self, conf,
		gensrv.RegisterInit(initSrv),
		gensrv.RegisterInfo(erl.DownMsg{}, handleDownMsg),
		gensrv.RegisterInfo(erl.ExitMsg{}, handleExitMsg),
	)
}

// Initialization function. Called when a process is started. Supervisors will block until this function returns.
func initSrv(self erl.PID, args any) (westate, any, error) {
	conf, ok := args.(weConfig)

	erl.ProcessFlag(self, erl.TrapExit, true)

	ref := erl.Monitor(self, conf.waitinOn)

	if !ok {
		return westate{}, nil, exitreason.Exception(errors.New("init arg must be a weConfig{}"))
	}
	return westate{conf: conf, ref: ref}, nil, nil
}

func handleDownMsg(self erl.PID, msg erl.DownMsg, state westate) (westate, any, error) {
	if msg.Proc.Equals(state.conf.waitinOn) && msg.Ref == state.ref {
		slog.Info("exitwaiter caught DownMsg, closing signal channel", "msg", msg)
		state.conf.wg.Done()
		return state, nil, exitreason.Normal
	} else {
		slog.Warn("exitwaiter caught DownMsg, but ref or proc did not match",
			"expected_ref", state.ref, "expected_proc", state.conf.waitinOn,
			"received_down_msg", msg)
	}
	return state, nil, nil
}

func handleExitMsg(self erl.PID, msg erl.ExitMsg, state westate) (westate, any, error) {
	if msg.Proc.Equals(state.conf.waitinOn) {
		slog.Info("waitExit caught ExitMsg, closing signal channel", "msg", msg)
		state.conf.wg.Done()
		return state, nil, exitreason.Normal
	}
	return state, nil, nil
}
