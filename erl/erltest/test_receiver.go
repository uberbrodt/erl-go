// this package contains the [TestReceiver] which is a process that
// can have message expectations set on them. These expectations match
// messages sent to the process inbox and execute a [TestExpectation] function. The
// function returns true to pass and false to fail the test.
package erltest

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

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

// Creates a new TestReceiver, which is a process that you can set
// message matching expectations on.
func NewReceiver(t *testing.T, opts ...ReceiverOpt) (erl.PID, *TestReceiver) {
	rOpts := receiverOptions{
		timeout:     DefaultReceiverTimeout,
		waitTimeout: DefaultWaitTimeout,
	}
	for _, o := range opts {
		rOpts = o(rOpts)
	}
	expectations := make(map[reflect.Type]*Expectation)
	castExpects := make(map[reflect.Type]*Expectation)
	callExpects := make(map[reflect.Type]*Expectation)
	tr := &TestReceiver{t: t, expectations: expectations, castExpects: castExpects, callExpects: callExpects, opts: rOpts}
	pid := erl.Spawn(tr)

	erl.ProcessFlag(pid, erl.TrapExit, true)
	t.Cleanup(func() {
		erl.Exit(erl.RootPID(), pid, exitreason.TestExit)
	})
	return pid, tr
}

type TestReceiver struct {
	t            *testing.T
	expectations map[reflect.Type]*Expectation
	castExpects  map[reflect.Type]*Expectation
	callExpects  map[reflect.Type]*Expectation
	failures     []*ExpectationFailure
	msgCnt       int
	self         erl.PID
	// if set to true, t.FailNow will not be called in [Pass] or [Wait]
	noFail    bool
	mx        sync.RWMutex
	testEnded bool
	opts      receiverOptions
}

func (tr *TestReceiver) Receive(self erl.PID, inbox <-chan any) error {
	tr.self = self
	for {
		select {
		case msg, ok := <-inbox:
			if !ok {
				return exitreason.Normal
			}
			var isXit bool
			switch v := msg.(type) {
			case erl.ExitMsg:
				isXit = true
				if errors.Is(v.Reason, exitreason.TestExit) {
					// NOTE: don't log exitmsg, it will cause a panic
					return exitreason.Normal
				}
			}
			// XXX: This is dumb. Race detector screeches if Logf is called outside of the test goroutine or
			// after the test is over.
			if !isXit {
				tr.t.Logf("TestReceiver got message: %#v", msg)
			}
			tr.mx.Lock()
			tr.check(msg)
			tr.mx.Unlock()
		case <-time.After(tr.opts.timeout):
			tr.t.Fatal("TestReceiver: test timeout")

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
				fail := tr.checkMatch(match, v.Msg, ex)
				if fail != nil {
					tr.failures = append(tr.failures, fail)
				}
			}
		}
	case genserver.CallRequest:
		callMsgT := reflect.TypeOf(v.Msg)
		for match, ex := range tr.callExpects {
			if callMsgT == match {
				// wrap our callHandle so we don't have to rewrite doCheck
				ex.h = func(self erl.PID, msg any) bool {
					return ex.callHandle(self, v.From, msg)
				}
				// pass in the unwrapped message
				fail := tr.checkMatch(match, v.Msg, ex)
				if fail != nil {
					tr.failures = append(tr.failures, fail)
				}
			}
		}
	default:
		msgT := reflect.TypeOf(msg)
		for match, ex := range tr.expectations {
			if msgT == match {
				fail := tr.checkMatch(match, msg, ex)
				if fail != nil {
					tr.failures = append(tr.failures, fail)
				}
			}
		}

	}
}

func (tr *TestReceiver) checkMatch(match reflect.Type, msg any, ex *Expectation) (failure *ExpectationFailure) {
	arg := ExpectArg{Match: match, Msg: msg, Self: tr.self, MsgCount: tr.msgCnt}
	return ex.Check(arg)
}

// Set an expectation that will be matched whenever an [expected] msg type is received.
func (tr *TestReceiver) Expect(expected any, handler TestExpectation, opts ...ExpectOpt) {
	t := reflect.TypeOf(expected)
	tr.expectations[t] = NewExpectation(handler, opts...)
}

// This is like [Expect] but is only tested against [genserver.CastRequest] messages.
func (tr *TestReceiver) ExpectCast(expected any, handler TestExpectation, opts ...ExpectOpt) {
	t := reflect.TypeOf(expected)
	tr.castExpects[t] = NewExpectation(handler, opts...)
}

// This is like [Expect] but is only tested against [genserver.CallRequest] messages.
// NOTE: You should use [genserver.Reply] to send a response to the [genserver.From], otherwise
// the caller will timeout
func (tr *TestReceiver) ExpectCall(expected any, handler TestCallExpectation, opts ...ExpectOpt) {
	t := reflect.TypeOf(expected)
	tr.callExpects[t] = NewCallExpectation(handler, opts...)
}

// returns the number of failed expectations and whether
// all expectations have been satisifed. An expectation is
// not satisfied unless it is invoked in the correct time and order
func (tr *TestReceiver) Pass() (int, bool) {
	tr.mx.RLock()
	defer tr.mx.RUnlock()
	expects := maps.Values(tr.expectations)
	castExpects := maps.Values(tr.castExpects)
	callExpects := maps.Values(tr.callExpects)
	reducer := func(list []*Expectation) bool {
		return fun.Reduce(list, true, func(v *Expectation, acc bool) bool {
			tr.t.Logf("checking %s times: %d matchCnt: %d is satisifed: %t", v.opts.exType, v.opts.times, v.matchCnt, v.satisfied)
			if !acc {
				return acc
			}

			return v.Satisified(tr.testEnded)
		})
	}

	// pass := check.Chain(tr.t, reducer(expects), reducer(castExpects), reducer(callExpects))
	return len(tr.failures), reducer(expects) && reducer(castExpects) && reducer(callExpects)
}

func (tr *TestReceiver) finish() bool {
	fails, passed := tr.Pass()
	if fails > 0 {
		tr.mx.Lock()
		defer tr.mx.Unlock()
		tr.t.Logf("TestReceiver %s, had %d expectation failures\r", tr.self, fails)
		for i, f := range tr.failures {
			tr.t.Logf("Failure %d: [Reason: %s] [MatchType: %+v] [Msg: %+v]", i, f.Reason, f.MatchType, f.Msg)
		}
		// tr.t.Logf("Failures: %+v", tr.failures)
		if !tr.noFail {
			tr.t.FailNow()
		} else {
			return false
		}
	}
	return passed
}

// Returns when the [WaitTimeout] expires or an expectation fails. If at the end of the timeout not
// all expectations are satisifed, the test is failed. Call this after you have sent your messages
// and want fail if your expecations don't pass
func (tr *TestReceiver) Wait() {
	now := time.Now()
	for {
		if time.Since(now) > tr.opts.waitTimeout {
			tr.t.Logf("test ended, checking expectations a final time")
			tr.testEnded = true
			passed := tr.finish()
			if !tr.noFail && !passed {
				tr.t.Fatal("TestReceiver.Loop test timeout")
			}
			return
		}

		// no failures and all passed, return so test ends
		passed := tr.finish()
		if passed {
			return
		}
		time.Sleep(time.Millisecond)
	}
}

// will cause the test receiver to exit, sending signals to linked and monitoring
// processes. This is not needed for normal test cleanup (that is handled via [t.Cleanup()])
func (tr *TestReceiver) Stop(self erl.PID) {
	erl.Exit(self, tr.self, exitreason.TestExit)
}
