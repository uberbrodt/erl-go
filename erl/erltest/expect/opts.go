package expect

import "github.com/uberbrodt/erl-go/erl/erltest"

type expectOpts struct {
	times  int
	exType exType
	tr     *erltest.TestReceiver
	name   string
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

// Set the TestReceiver that the expectation will add itself to.
// Useful when you are registering [Simple] sub-expectations
func Receiver(tr *erltest.TestReceiver) ExpectOpt {
	return func(o expectOpts) expectOpts {
		o.tr = tr
		return o
	}
}

// Set a name that will identify the Expectation in error reports and logs
func Name(name string) ExpectOpt {
	return func(o expectOpts) expectOpts {
		o.name = name
		return o
	}
}
