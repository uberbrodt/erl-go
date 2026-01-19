package expect

import (
	"fmt"

	"github.com/uberbrodt/erl-go/erl/erltest"
)

type Expectation struct {
	h         Handle
	opts      ExpectOpts
	matchCnt  int
	satisfied bool
	id        string
	children  []*Expectation
	name      string
}

// Returns true if this and all joined exceptions are satisifed,
// and immediately if it encounters a failure.
//
// If [testEnded] is true, checks that should never be executed (ie: Times(0)) will be evaluated.
func (ex *Expectation) Satisfied(waitExpired bool) bool {
	for _, child := range ex.children {
		if ok := doSatisifed(child, waitExpired); !ok {
			return false
		}
	}
	return doSatisifed(ex, waitExpired)
}

func doSatisifed(ex *Expectation, waitExpired bool) bool {
	// these two types can only be evaluated at test end
	if (ex.opts.exType == exact || ex.opts.exType == atMost) && !waitExpired {
		return false
	}

	if ex.opts.exType == exact && ex.opts.times == 0 {
		return ex.matchCnt == 0
	} else if ex.opts.exType == atMost {
		return ex.matchCnt <= ex.opts.times
	} else {
		return ex.satisfied
	}
}

// Joins [ex] with the [exs], so that all [exs] must succeed
// in order for [ex.Satisfied] to pass.
// WARNING: any [*Expectation] returned from an[exs] is ignored. This prevents branching, which
// can be accomplished by composing exceptions inside a [Handle].
func (ex *Expectation) And(exs ...*Expectation) {
	for _, h := range exs {
		// replace the opts since children are called the exact same number of times
		h.opts = ex.opts
		ex.children = append(ex.children, h)
	}
}

// [Check] should be executed after a "match" has been made to the expectation. It will
// return an error if it has been clearly executed too many times. If you want to evaluate
// if an Expectation has "passed", you should call [Satisified].
func (ex *Expectation) Check(arg erltest.ExpectArg) (next erltest.Expectation, failure *erltest.ExpectationFailure) {
	defer func() {
		if r := recover(); r != nil {
			failure = Fail(arg, fmt.Sprintf("TestExpectation panicked!: %+v", r))
		}
	}()
	ex.matchCnt = ex.matchCnt + 1

	// look for failures
	switch ex.opts.exType {
	case absolute:
		// if we are matching absolute order, fail if the opts.times != the nth msg received
		if arg.MsgCount != ex.opts.times {
			return nil, Fail(arg, fmt.Sprintf("expected to match msg #%d, but matched with msg #%d", ex.opts.times, arg.MsgCount))
		}
	case exact:
		if ex.matchCnt > ex.opts.times {
			ex.satisfied = false
			return nil, Fail(arg, fmt.Sprintf("expected to match %d times, but match count is now: %d", ex.opts.times, ex.matchCnt))
		}
	case atMost:
		if ex.matchCnt > ex.opts.times {
			return nil, Fail(arg, fmt.Sprintf("expected to match at most %d times, but match count is now: %d", ex.opts.times, ex.matchCnt))
		}
	}

	// if we returned false on a match, this is a failure. Otherwise, returns a nested expectation
	if next, failure = ex.h(arg); failure != nil {
		return next, failure
	}
	for _, e := range ex.children {
		if _, failure = e.Check(arg); failure != nil {
			return next, failure
		}
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

	return next, failure
}

func (ex *Expectation) Name() string {
	return ex.name
}

func (ex *Expectation) String() string {
	name := ex.name
	if ex.name == "" {
		name = ex.id
	}
	return fmt.Sprintf("Expectation[%s]{type: %s, expected_times: %d, matches: %d, satisfied: %t, satisifed(end-of-test): %t}",
		name,
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
