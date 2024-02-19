package erltest

import (
	"fmt"
	"reflect"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/genserver"
)

type (
	TestExpectation     func(self erl.PID, msg any) bool
	TestCallExpectation func(self erl.PID, from genserver.From, msg any) bool
)

type Expectation struct {
	h          TestExpectation
	callHandle TestCallExpectation
	opts       expectOpts
	matchCnt   int
	satisfied  bool
}

type ExpectArg struct {
	Match    reflect.Type
	Msg      any
	Self     erl.PID
	MsgCount int
}

func NewExpectation(te TestExpectation, opts ...ExpectOpt) *Expectation {
	o := expectOpts{times: 1, exType: exact}

	for _, f := range opts {
		o = f(o)
	}

	return &Expectation{
		h:         te,
		opts:      o,
		satisfied: o.exType == anyTimes,
	}
}

func NewCallExpectation(te TestCallExpectation, opts ...ExpectOpt) *Expectation {
	o := expectOpts{times: 1, exType: exact}

	for _, f := range opts {
		o = f(o)
	}

	return &Expectation{
		callHandle: te,
		opts:       o,
		satisfied:  o.exType == anyTimes,
	}
}

// Returns the result of the expectation. If [testEnded] is true, checks that
// should never be executed (ie: Times(0)) will be evaluated.
func (ex *Expectation) Satisified(testEnded bool) bool {
	if ex.opts.exType == exact && ex.opts.times == 0 && testEnded {
		return ex.matchCnt == 0
	} else if ex.opts.exType == atMost && testEnded {
		return ex.matchCnt <= ex.opts.times
	} else {
		return ex.satisfied
	}
}

// [Check] should be executed after a "match" has been made to the expectation. It will
// return an error if it has been clearly executed too many times. If you want to evaluate
// if an Expectation has "passed", you should call [Satisified].
func (ex *Expectation) Check(arg ExpectArg) (failure *ExpectationFailure) {
	defer func() {
		if r := recover(); r != nil {
			failure = &ExpectationFailure{
				MatchType: arg.Match,
				Msg:       arg.Msg,
				Reason:    fmt.Sprintf("TestExpectation panicked!: %+v", r),
			}
		}
	}()
	ex.matchCnt = ex.matchCnt + 1

	// look for failures
	switch ex.opts.exType {
	case absolute:
		// if we are matching absolute order, fail if the opts.times != the nth msg received
		if arg.MsgCount != ex.opts.times {
			return &ExpectationFailure{
				MatchType: arg.Match,
				Msg:       arg.Msg,
				Reason:    fmt.Sprintf("expected to match msg #%d, but matched with msg #%d", ex.opts.times, arg.MsgCount),
			}
		}
	case exact:
		if ex.matchCnt > ex.opts.times {
			ex.satisfied = false
			return &ExpectationFailure{
				MatchType: arg.Match,
				Msg:       arg.Msg,
				Reason:    fmt.Sprintf("expected to match %d times, but match count is now: %d", ex.opts.times, ex.matchCnt),
			}
		}
	case atMost:
		if ex.matchCnt > ex.opts.times {
			return &ExpectationFailure{
				MatchType: arg.Match,
				Msg:       arg.Msg,
				Reason:    fmt.Sprintf("expected to match at most %d times, but match count is now: %d", ex.opts.times, ex.matchCnt),
			}
		}
	}

	// if we're not matching any # of times and our matchCnt doesn't equal the times called, fail. This assumes
	// we've already incremented the matchCnt

	// if we returned false on a match, this is a failure.
	if result := ex.h(arg.Self, arg.Msg); !result {
		return &ExpectationFailure{MatchType: arg.Match, Msg: arg.Msg, Reason: "returned false"}
	}

	// mark satisifed
	switch ex.opts.exType {
	case absolute:
		if arg.MsgCount == ex.opts.times {
			ex.satisfied = true
		}
	case exact:
		// we wait until end of test to evalute if we've been called zero times.
		if ex.opts.times > 0 {
			ex.satisfied = ex.opts.times == ex.matchCnt
		}
	case atLeast:
		ex.satisfied = ex.matchCnt >= ex.opts.times
	}

	return failure
}

type ExpectationFailure struct {
	MatchType reflect.Type
	Msg       any
	Reason    string
}

type expectOpts struct {
	times  int
	exType exType
}

type exType string

const (
	exact    exType = "EXACT"
	atMost   exType = "AT_MOST"
	atLeast  exType = "AT_LEAST"
	absolute exType = "ABSOLUTE"
	anyTimes exType = "ANY_TIMES"
)

type ExpectOpt func(o expectOpts) expectOpts

// Expectation is satisifed only if it is invoed [n] times.
func Times(n int) ExpectOpt {
	return func(o expectOpts) expectOpts {
		o.times = n
		o.exType = exact
		return o
	}
}

// Expectation is satisifed if it is executed up to [n] times. Zero executions will also pass
// WARNING: this expectation will cause your test to run until [WaitTimeout] is exceeded
func AtMost(n int) ExpectOpt {
	return func(o expectOpts) expectOpts {
		o.times = n
		o.exType = atMost
		return o
	}
}

// Expectation is satisifed if it is executed zero or more times.
// Ensure that you are sleeping the test or have other expectations so [TestReciever.Wait]
// does not exit immediately
func AnyTimes() ExpectOpt {
	return func(o expectOpts) expectOpts {
		o.exType = anyTimes
		return o
	}
}

// specify that an expectation should match the Nth msg received by the TestReceiver
func Absolute(nthMsg int) ExpectOpt {
	return func(o expectOpts) expectOpts {
		o.exType = absolute
		o.times = nthMsg

		return o
	}
}

// expectation will pass only if matched >=[n] times.
func AtLeast(n int) ExpectOpt {
	return func(o expectOpts) expectOpts {
		o.exType = atLeast
		o.times = n

		return o
	}
}

// expectation will pass only if never matched. Alias for `Times(0)`
// WARNING: this expectation will cause your test to run until [WaitTimeout] is exceeded
func Never() ExpectOpt {
	return func(o expectOpts) expectOpts {
		o.exType = exact
		o.times = 0

		return o
	}
}
