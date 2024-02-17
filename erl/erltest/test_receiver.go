// this package contains the [TestReceiver] which is a process that
// can have message expectations set on them. These expectations match
// messages sent to the process inbox and execute a [TestExpectation] function. The
// function returns true to pass and false to fail the test.
package erltest

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/uberbrodt/fungo/fun"
	"golang.org/x/exp/maps"

	"github.com/uberbrodt/erl-go/chronos"
	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
)

var testTimeout time.Duration = chronos.Dur("10s")

type expectation struct {
	h         TestExpectation
	opts      expectOpts
	matchCnt  int
	satisfied bool
}

// Creates a new TestReceiver, which is a process that you can set
// message matching expectations on.
func NewReceiver(t *testing.T) (erl.PID, *TestReceiver) {
	expectations := make(map[reflect.Type]*expectation)
	tr := &TestReceiver{t: t, expectations: expectations}
	pid := erl.Spawn(tr)

	erl.ProcessFlag(pid, erl.TrapExit, true)
	t.Cleanup(func() {
		erl.Exit(erl.RootPID(), pid, exitreason.TestExit)
	})
	return pid, tr
}

type TestReceiver struct {
	t            *testing.T
	expectations map[reflect.Type]*expectation
	failures     []*ExpectationFailure
	msgCnt       int
	self         erl.PID
	noFail       bool
	mx           sync.RWMutex
}

func (tr *TestReceiver) Receive(self erl.PID, inbox <-chan any) error {
	tr.self = self
	for {
		select {
		case msg, ok := <-inbox:
			if !ok {
				return exitreason.Normal
			}
			switch v := msg.(type) {
			case erl.ExitMsg:
				if errors.Is(v.Reason, exitreason.TestExit) {
					// NOTE: don't log exitmsg, it will cause a panic
					return exitreason.Normal
				}
			}
			tr.t.Logf("TestReceiver got message: %#v", msg)
			tr.mx.Lock()
			tr.check(msg)
			tr.mx.Unlock()
		case <-time.After(testTimeout):
			tr.t.Fatal("TestReceiver: test timeout")

			return exitreason.Timeout
		}
	}
}

func (tr *TestReceiver) check(msg any) {
	tr.msgCnt = tr.msgCnt + 1
	msgT := reflect.TypeOf(msg)

	for match, ex := range tr.expectations {
		if msgT == match {
			fail := tr.doCheck(match, msg, ex)
			if fail != nil {
				tr.failures = append(tr.failures, fail)
			}
		}
	}
}

func (tr *TestReceiver) doCheck(match reflect.Type, msg any, ex *expectation) *ExpectationFailure {
	ex.matchCnt = ex.matchCnt + 1

	// look for failures
	switch ex.opts.exType {
	case absolute:
		// if we are matching absolute order, fail if the opts.times != the nth msg received
		if tr.msgCnt != ex.opts.times {
			return &ExpectationFailure{
				MatchType: match,
				Msg:       msg,
				Reason:    fmt.Sprintf("expected to match msg #%d, but matched with msg #%d", ex.opts.times, tr.msgCnt),
			}
		}
	case exact:
		if ex.matchCnt != ex.opts.times {
			return &ExpectationFailure{
				MatchType: match,
				Msg:       msg,
				Reason:    fmt.Sprintf("expected to match %d times, but match count is now: %d", ex.opts.times, ex.matchCnt),
			}
		}
	case atMost:
		if ex.matchCnt > ex.opts.times {
			return &ExpectationFailure{
				MatchType: match,
				Msg:       msg,
				Reason:    fmt.Sprintf("expected to match at most %d times, but match count is now: %d", ex.opts.times, ex.matchCnt),
			}
		}
	}

	// if we're not matching any # of times and our matchCnt doesn't equal the times called, fail. This assumes
	// we've already incremented the matchCnt

	// if we returned false on a match, this is a failure.
	if result := ex.h(tr.self, msg); !result {
		return &ExpectationFailure{MatchType: match, Msg: msg, Reason: "returned false"}
	}

	// mark satisifed
	switch ex.opts.exType {
	case absolute:
		if tr.msgCnt == ex.opts.times {
			ex.satisfied = true
		}
	case exact:
		ex.satisfied = ex.opts.times == ex.matchCnt
	case atLeast:
		ex.satisfied = ex.matchCnt >= ex.opts.times
	}

	return nil
}

// Set an expectation that will be matched whenever an [expected] msg type is received.
func (tr *TestReceiver) Expect(expected any, handler TestExpectation, opts ...ExpectOpt) {
	o := expectOpts{times: 1, exType: exact}

	for _, f := range opts {
		o = f(o)
	}

	t := reflect.TypeOf(expected)
	tr.expectations[t] = &expectation{
		h:         handler,
		opts:      o,
		satisfied: o.exType == anyTimes || o.exType == atMost,
	}
}

// returns the number of failed expectations and whether
// all expectations have been satisifed. An expectation is
// not satisfied unless it is invoked in the correct time and order
func (tr *TestReceiver) Pass() (int, bool) {
	tr.mx.RLock()
	defer tr.mx.RUnlock()
	expects := maps.Values(tr.expectations)
	pass := fun.Reduce(expects, true, func(v *expectation, acc bool) bool {
		tr.t.Logf("checking %s times: %d matchCnt: %d is satisifed: %t", v.opts.exType, v.opts.times, v.matchCnt, v.satisfied)
		if !acc {
			return acc
		}

		return v.satisfied
	})
	return len(tr.failures), pass
}

// Returns when the [tout] expires or an expectation fails. If at the end of the timeout not
// all expectations are satisifed, the test is failed. Call this after you have sent your messages
// and want fail if your expecations don't pass
func (tr *TestReceiver) WaitFor(tout time.Duration) {
	now := time.Now()
	for {
		if time.Since(now) > tout {
			if !tr.noFail {
				tr.t.Fatal("TestReceiver.Loop test timeout")
			} else {
				return
			}
		}

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
				return
			}
		}
		// no failures and all passed, return so test ends
		if passed {
			return
		}
		time.Sleep(time.Millisecond)
	}
}

// same as [WaitFor], but uses default test timeout
func (tr *TestReceiver) Wait() {
	tr.WaitFor(testTimeout)
}
