package erltest

import (
	"reflect"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/genserver"
)

type (
	TestExpectation     func(self erl.PID, msg any) bool
	TestCallExpectation func(self erl.PID, from genserver.From, msg any) bool
)

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

// Expectation is satisifed if it is executed up to [n] times. Zero
// executions will also pass, so ensure that you are sleeping the test or have
// other expectations so [TestReciever.Wait] does not exit immediately
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
