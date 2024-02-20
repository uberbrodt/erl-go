package expect

import (
	"fmt"

	"github.com/uberbrodt/erl-go/erl/erltest"
)

type Expectation struct {
	h Handle
	// h          BoolTestExpectation
	// callHandle BoolTestCallExpectation
	opts      expectOpts
	matchCnt  int
	satisfied bool
	id        string
}

// Returns the result of the expectation. If [testEnded] is true, checks that
// should never be executed (ie: Times(0)) will be evaluated.
func (ex *Expectation) Satisfied(testEnded bool) bool {
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
func (ex *Expectation) Check(arg erltest.ExpectArg) (next erltest.Expectation, failure *erltest.ExpectationFailure) {
	defer func() {
		if r := recover(); r != nil {
			failure = &erltest.ExpectationFailure{
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
			return nil, &erltest.ExpectationFailure{
				MatchType: arg.Match,
				Msg:       arg.Msg,
				Reason:    fmt.Sprintf("expected to match msg #%d, but matched with msg #%d", ex.opts.times, arg.MsgCount),
			}
		}
	case exact:
		if ex.matchCnt > ex.opts.times {
			ex.satisfied = false
			return nil, &erltest.ExpectationFailure{
				MatchType: arg.Match,
				Msg:       arg.Msg,
				Reason:    fmt.Sprintf("expected to match %d times, but match count is now: %d", ex.opts.times, ex.matchCnt),
			}
		}
	case atMost:
		if ex.matchCnt > ex.opts.times {
			return nil, &erltest.ExpectationFailure{
				MatchType: arg.Match,
				Msg:       arg.Msg,
				Reason:    fmt.Sprintf("expected to match at most %d times, but match count is now: %d", ex.opts.times, ex.matchCnt),
			}
		}
	}

	if next, failure = ex.h(arg); failure != nil {
		return
	}
	// if we returned false on a match, this is a failure.
	// if result := ex.h(arg.Self, arg.Msg); !result {
	// 	return &ExpectationFailure{MatchType: arg.Match, Msg: arg.Msg, Reason: "returned false"}
	// }

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

	return next, failure
}

func (ex *Expectation) String() string {
	return fmt.Sprintf("Expectation{type: %s, expected_times: %d, matches: %d, satisfied: %t, satisifed(end-of-test): %t}",
		ex.opts.exType,
		ex.opts.times,
		ex.matchCnt,
		ex.Satisfied(false),
		ex.Satisfied(true),
	)
}

func (ex *Expectation) ID() string {
	return ex.id
}
