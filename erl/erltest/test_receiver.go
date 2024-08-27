// this package contains the [TestReceiver] which is a process that
// can have message expectations set on them. These expectations match
// messages sent to the process inbox and execute a [TestExpectation] function. The
// function returns true to pass and false to fail the test.
package erltest

import (
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/rs/xid"
	"github.com/uberbrodt/fungo/fun"
	"golang.org/x/exp/maps"

	"github.com/uberbrodt/erl-go/chronos"
	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
	"github.com/uberbrodt/erl-go/erl/genserver"
)

var (
	DefaultReceiverTimeout time.Duration = chronos.Dur("10s")
	DefaultWaitTimeout     time.Duration = chronos.Dur("5s")
)

type ReceiverOpt func(ro receiverOptions) receiverOptions

type receiverOptions struct {
	timeout     time.Duration
	waitTimeout time.Duration
	noFail      bool
	name        string
	logger      *slog.Logger
}

// Specify how long the test reciever should run for before stopping.
// this needs to be set otherwise tests will hang until exceptions are matched or
// the 10min Go default is reached. See [DefaultReceiverTimeout]
func ReceiverTimeout(t time.Duration) ReceiverOpt {
	return func(ro receiverOptions) receiverOptions {
		ro.timeout = t
		return ro
	}
}

// Specify how long for [TestReciever.Wait] for all expectations to be met
// see [DefaultWaitTimeout]
func WaitTimeout(t time.Duration) ReceiverOpt {
	return func(ro receiverOptions) receiverOptions {
		ro.waitTimeout = t
		return ro
	}
}

// Set this if you do not want the TestReceiver to call [testing.T.Fail] in
// the [Wait] method.
func NoFail() ReceiverOpt {
	return func(ro receiverOptions) receiverOptions {
		ro.noFail = true
		return ro
	}
}

// Set a name for the test receiver, that will be used in log messages
func Name(name string) ReceiverOpt {
	return func(ro receiverOptions) receiverOptions {
		ro.name = name
		return ro
	}
}

// XXX: might remove.
func SetLogger(logger *slog.Logger) ReceiverOpt {
	return func(ro receiverOptions) receiverOptions {
		ro.logger = logger
		return ro
	}
}

// Creates a new TestReceiver, which is a process that you can set
// message matching expectations on.
func NewReceiver(t *testing.T, opts ...ReceiverOpt) (erl.PID, *TestReceiver) {
	rOpts := receiverOptions{
		timeout:     DefaultReceiverTimeout,
		waitTimeout: DefaultWaitTimeout,
		name:        fmt.Sprintf("%s-test-receiver", xid.New().String()),
	}
	for _, o := range opts {
		rOpts = o(rOpts)
	}
	expectations := make(map[reflect.Type]Expectation)
	castExpects := make(map[reflect.Type]Expectation)
	callExpects := make(map[reflect.Type]Expectation)
	allExpects := make(map[string]Expectation)

	tr := &TestReceiver{
		t: t, msgExpects: expectations, castExpects: castExpects, callExpects: callExpects,
		opts: rOpts, noFail: rOpts.noFail, allExpects: allExpects,
	}
	if rOpts.logger != nil {
		tr.log = rOpts.logger
	} else {
		tr.log = slog.With("erltest.test-receiver", rOpts.name)
	}
	pid := erl.Spawn(tr)

	erl.ProcessFlag(pid, erl.TrapExit, true)
	t.Cleanup(func() {
		tr.log.Info("shutting down...")
		erl.Exit(erl.RootPID(), pid, exitreason.TestExit)
	})
	return pid, tr
}

type TestReceiver struct {
	t           *testing.T
	msgExpects  map[reflect.Type]Expectation
	castExpects map[reflect.Type]Expectation
	callExpects map[reflect.Type]Expectation
	allExpects  map[string]Expectation
	failures    []*ExpectationFailure
	msgCnt      int
	self        erl.PID
	// if set to true, t.FailNow will not be called in [Pass] or [Wait]
	noFail    bool
	mx        sync.RWMutex
	selfmx    sync.RWMutex
	testEnded bool
	opts      receiverOptions
	exiting   bool
	log       *slog.Logger
}

func (tr *TestReceiver) getSelf() erl.PID {
	defer tr.selfmx.RUnlock()
	tr.selfmx.RLock()
	return tr.self
}

func (tr *TestReceiver) setSelf(pid erl.PID) {
	defer tr.selfmx.Unlock()
	tr.selfmx.Lock()
	tr.self = pid
}

// only log if the test isn't existing or ended.
func (tr *TestReceiver) safeTLogf(format string, args ...any) {
	if !tr.exiting {
		prefix := fmt.Sprintf("[TestReceiver: %s|PID: %+v]: %s", tr.opts.name, tr.getSelf(), format)
		tr.t.Logf(prefix, args...)
	}
}

// log and mark the test as failed
func (tr *TestReceiver) safeTError(format string, args ...any) {
	prefix := fmt.Sprintf("[TestReceiver: %s|PID: %+v]: %s", tr.opts.name, tr.getSelf(), format)
	tr.t.Errorf(prefix, args...)
}

func (tr *TestReceiver) Receive(self erl.PID, inbox <-chan any) error {
	tr.setSelf(self)
	// tr.self = self
	for {
		select {
		case msg, ok := <-inbox:
			if !ok {
				return exitreason.Normal
			}
			switch v := msg.(type) {
			case erl.ExitMsg:
				tr.exiting = true
				if errors.Is(v.Reason, exitreason.TestExit) {
					tr.log.Info("recieved a TestExit, shutting down", "reason", v.Reason, "sending-proc", v.Proc)
					return exitreason.Normal
				}
			}
			tr.safeTLogf("TestReceiver got message: %#v", msg)

			tr.mx.Lock()
			tr.check(msg)
			tr.mx.Unlock()
		case <-time.After(tr.opts.timeout):
			tr.safeTError("TestReceiver: test timeout")

			return exitreason.Timeout
		}
	}
}

func (tr *TestReceiver) check(msg any) {
	tr.msgCnt = tr.msgCnt + 1

	switch v := msg.(type) {
	case genserver.CastRequest:
		castMsgT := reflect.TypeOf(v.Msg)
		for match, ex := range tr.castExpects {
			if castMsgT == match {
				// pass in the unwrapped message
				fail := tr.checkMatch(match, v.Msg, nil, ex)
				if fail != nil {
					tr.failures = append(tr.failures, fail)
				}
			}
		}
	case genserver.CallRequest:
		callMsgT := reflect.TypeOf(v.Msg)
		for match, ex := range tr.callExpects {
			if callMsgT == match {
				fail := tr.checkMatch(match, v.Msg, &v.From, ex)
				if fail != nil {
					tr.failures = append(tr.failures, fail)
				}
			}
		}
	default:
		msgT := reflect.TypeOf(msg)
		for match, ex := range tr.msgExpects {
			if msgT == match {
				fail := tr.checkMatch(match, msg, nil, ex)
				if fail != nil {
					tr.failures = append(tr.failures, fail)
				}
			}
		}

	}
}

func (tr *TestReceiver) checkMatch(match reflect.Type, msg any, from *genserver.From, ex Expectation) (failure *ExpectationFailure) {
	arg := ExpectArg{Match: match, Exp: ex, Msg: msg, Self: tr.getSelf(), MsgCount: tr.msgCnt, From: from}
	if nextEx, fail := ex.Check(arg); fail != nil {
		return fail
	} else if nextEx != nil {
		return tr.checkMatch(match, msg, from, nextEx)
	} else {
		return nil
	}
}

// Register an expectation with this TestReciever. It will be checked
// when Pass is called (and as a consequnce, cause [Wait] to block until its success)
func (tr *TestReceiver) WaitOn(e ...Expectation) {
	for _, ex := range e {
		tr.allExpects[ex.ID()] = ex
	}
}

// Set an expectation that will be matched whenever a [matchTerm] msg type is received.
func (tr *TestReceiver) Expect(matchTerm any, e Expectation) {
	t := reflect.TypeOf(matchTerm)
	tr.allExpects[e.ID()] = e
	tr.msgExpects[t] = e
}

// This is like [Expect] but is only tested against [genserver.CastRequest] messages.
func (tr *TestReceiver) ExpectCast(matchTerm any, e Expectation) {
	t := reflect.TypeOf(matchTerm)
	tr.allExpects[e.ID()] = e
	tr.castExpects[t] = e
}

// This is like [Expect] but is only tested against [genserver.CallRequest] messages.
// NOTE: You should use [genserver.Reply] to send a response to the [genserver.From], otherwise
// the caller will timeout
func (tr *TestReceiver) ExpectCall(matchTerm any, e Expectation) {
	t := reflect.TypeOf(matchTerm)
	tr.allExpects[e.ID()] = e
	tr.callExpects[t] = e
}

// returns the number of failed expectations and whether
// all expectations have been satisifed. An expectation is
// not satisfied unless it is invoked in the correct time and order
func (tr *TestReceiver) Pass() (int, bool) {
	tr.mx.RLock()
	defer tr.mx.RUnlock()
	expects := maps.Values(tr.allExpects)
	reducer := func(list []Expectation) bool {
		return fun.Reduce(list, true, func(v Expectation, acc bool) bool {
			if !acc {
				return acc
			}

			ok := v.Satisfied(tr.testEnded)
			if !ok && tr.testEnded {
				tr.safeTLogf("%v is not satisfied", v)
			}
			return ok
		})
	}

	return len(tr.failures), reducer(expects)
}

func (tr *TestReceiver) finish() (done bool, failed bool) {
	fails, passed := tr.Pass()
	if fails > 0 {
		tr.mx.Lock()
		defer tr.mx.Unlock()
		tr.safeTLogf("%d expectation failures\r", fails)
		for i, f := range tr.failures {
			tr.safeTLogf("Failure %d: [Expect: %s] [Reason: %s] [Match: %v] [Msg: %+v]", i, f.Exp.Name(), f.Reason, f.Match, f.Msg)
		}
		if !tr.noFail {
			tr.t.Fail()
		} else {
			return false, true
		}
	}
	return passed, false
}

// Returns when the [WaitTimeout] expires or an expectation fails. If at the end of the timeout not
// all expectations are satisifed, the test is failed. Call this after you have sent your messages
// and want fail if your expecations don't pass
func (tr *TestReceiver) Wait() {
	now := time.Now()
	for {
		if time.Since(now) > tr.opts.waitTimeout {
			tr.safeTLogf("test ended, checking expectations a final time")
			tr.testEnded = true
			passed, _ := tr.finish()
			if !tr.noFail && !passed {
				tr.safeTError("Wait(): timeout reached")
			}
			return
		}

		// no failures and all passed, return so test ends
		passed, failed := tr.finish()
		if passed {
			return
		}
		if failed {
			return
		}
		time.Sleep(time.Millisecond)
	}
}

// will cause the test receiver to exit, sending signals to linked and monitoring
// processes. This is not needed for normal test cleanup (that is handled via [t.Cleanup()])
func (tr *TestReceiver) Stop(self erl.PID) {
	erl.Exit(self, tr.getSelf(), exitreason.TestExit)
}

func (tr *TestReceiver) Failures() []*ExpectationFailure {
	return tr.failures
}

// return the [*testing.T]. Don't use this in situations that could run after a
// test is failed, the race detector doesn't like that.
func (tr *TestReceiver) T() *testing.T {
	return tr.t
}
