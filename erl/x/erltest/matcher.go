package erltest

import "fmt"

// A Matcher is a representation of a class of values.
// It is used to represent the valid or expected arguments to a mocked method.
//
// This interface is compatible with the one gomock uses, so methods like [gomock.Eq] are
// valid matches that can be used when creating an [Expectation]
type Matcher interface {
	// Matches returns whether x is a match.
	Matches(x any) bool

	// String describes what the matcher matches.
	String() string
}

// GotFormatter is used to better print failure messages. If a matcher
// implements GotFormatter, it will use the result from Got when printing
// the failure message.
type GotFormatter interface {
	// Got is invoked with the received value. The result is used when
	// printing the failure message.
	Got(got any) string
}

func formatGottenArg(m Matcher, arg any) string {
	got := fmt.Sprintf("%v (%T)", arg, arg)
	if gs, ok := m.(GotFormatter); ok {
		got = gs.Got(arg)
	}
	return got
}
