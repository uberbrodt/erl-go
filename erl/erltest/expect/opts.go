package expect

import "github.com/uberbrodt/erl-go/erl/erltest"

type ExpectOpts struct {
	times  int
	exType ExType
	tr     *erltest.TestReceiver
	name   string
}

type ExType string

const (
	exact    ExType = "EXACT"
	atMost   ExType = "AT_MOST"
	atLeast  ExType = "AT_LEAST"
	absolute ExType = "ABSOLUTE"
	anyTimes ExType = "ANY_TIMES"
)

// This is a transitional method to help porting from this package to `x/erltest`
func ParseOpts(opts ...ExpectOpt) ExpectOpts {
	o := ExpectOpts{times: 1, exType: atLeast}

	for _, f := range opts {
		o = f(o)
	}
	return o
}

type ExpectOpt func(o ExpectOpts) ExpectOpts

// get the number of times the expectation should match
// This is a transitional method to help porting from this package to `x/erltest`
func (eo *ExpectOpts) GetTimes(n int) int {
	return eo.times
}

// get the type of expectation.
// This is a transitional method to help porting from this package to `x/erltest`
func (eo *ExpectOpts) GetExType(n int) ExType {
	return eo.exType
}

// get the name of the expectation.
// This is a transitional method to help porting from this package to `x/erltest`
func (eo *ExpectOpts) GetName(n int) ExType {
	return eo.exType
}

// Expectation is satisifed only if it is invoked [n] times.
//
// NOTE: this is a blocking expectation, that will not be satisifed until
// the wait timeout is expired in the [erltest.TestReceiver]. If setting multiple
// expectations for the same msg, no subsequent expectations can match until after
// the wait timeout.
func Times(n int) ExpectOpt {
	return func(o ExpectOpts) ExpectOpts {
		o.times = n
		o.exType = exact
		return o
	}
}

// Expectation is satisifed if it is executed up to [n] times. Zero executions will also pass
//
// NOTE: this is a blocking expectation, that will not be satisifed until
// the wait timeout is expired in the [erltest.TestReceiver]. If setting multiple
// expectations for the same msg, no subsequent expectations can match until after
// the wait timeout.
func AtMost(n int) ExpectOpt {
	return func(o ExpectOpts) ExpectOpts {
		o.times = n
		o.exType = atMost
		return o
	}
}

// Expectation is satisifed if it is executed zero or more times.
// Ensure that you are sleeping the test or have other expectations so [TestReciever.Wait]
// does not exit immediately
func AnyTimes() ExpectOpt {
	return func(o ExpectOpts) ExpectOpts {
		o.exType = anyTimes
		return o
	}
}

// specify that an expectation should match the Nth msg received by the TestReceiver
func Absolute(nthMsg int) ExpectOpt {
	return func(o ExpectOpts) ExpectOpts {
		o.exType = absolute
		o.times = nthMsg

		return o
	}
}

// expectation will pass only if matched >=[n] times.
func AtLeast(n int) ExpectOpt {
	return func(o ExpectOpts) ExpectOpts {
		o.exType = atLeast
		o.times = n

		return o
	}
}

// expectation will pass only if never matched. Alias for `Times(0)`
//
// NOTE: this is a blocking expectation, that will not be satisifed until
// the wait timeout is expired in the [erltest.TestReceiver]. If setting multiple
// expectations for the same msg, no subsequent expectations can match until after
// the wait timeout.
func Never() ExpectOpt {
	return func(o ExpectOpts) ExpectOpts {
		o.exType = exact
		o.times = 0

		return o
	}
}

// Set the TestReceiver that the expectation will add itself to.
// Useful when you are registering [Simple] sub-expectations
func Receiver(tr *erltest.TestReceiver) ExpectOpt {
	return func(o ExpectOpts) ExpectOpts {
		o.tr = tr
		return o
	}
}

// Set a name that will identify the Expectation in error reports and logs
func Name(name string) ExpectOpt {
	return func(o ExpectOpts) ExpectOpts {
		o.name = name
		return o
	}
}
