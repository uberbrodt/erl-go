package expect

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/rs/xid"
	"github.com/uberbrodt/erl-go/erl/erltest"
	"github.com/uberbrodt/erl-go/erl/erltest/check"
)

type Handle func(erltest.ExpectArg) (erltest.Expectation, *erltest.ExpectationFailure)

// Create a custom expectation that can branch to other expectations
//
// IMPORTANT: Any expectation used in a handler needs to be [Attach]ed to
// a test receiver. If there are multiple possible expectations that can be
// returned here, they all need to be attached to make sure the [TestReceiver] does
// not pass without checking them.
func New(te Handle, opts ...ExpectOpt) *Expectation {
	o := expectOpts{times: 1, exType: exact}

	for _, f := range opts {
		o = f(o)
	}

	return new(te, o)
}

func new(te Handle, opts expectOpts) *Expectation {
	ex := &Expectation{
		id:        xid.New().String(),
		h:         te,
		opts:      opts,
		name:      opts.name,
		satisfied: opts.exType == anyTimes,
	}

	if opts.tr != nil {
		opts.tr.WaitOn(ex)
	}

	return ex
}

func Fail(ea erltest.ExpectArg, reason string) *erltest.ExpectationFailure {
	return erltest.Fail(ea, reason)
}

// Attach an expectation to a receiver. If the expectation is not satisfied, the receiver will
// not Pass.
func Attach(tr *erltest.TestReceiver, e erltest.Expectation) erltest.Expectation {
	tr.WaitOn(e)
	return e
}

// a simple terminal expectation that works well with [check.Chain]
func Expect(f func(arg erltest.ExpectArg) bool, opts ...ExpectOpt) *Expectation {
	return New(func(arg erltest.ExpectArg) (erltest.Expectation, *erltest.ExpectationFailure) {
		if ok := f(arg); !ok {
			return nil, Fail(arg, "returned false")
		}
		return nil, nil
	}, opts...)
}

// Set an expectation that will pass if it is matched (or not matched, depending on opts).
// Useful if we don't care about a message's contents
func Called(opts ...ExpectOpt) *Expectation {
	return New(func(arg erltest.ExpectArg) (erltest.Expectation, *erltest.ExpectationFailure) {
		return nil, nil
	}, opts...)
}

// Create an [Equals] expectation and attach it to [tr]
func AttachEquals(tr *erltest.TestReceiver, expected any, opts ...ExpectOpt) *Expectation {
	opts = append(opts, Receiver(tr))
	return Equals(tr.T(), expected, opts...)
}

// assert a message equals exactly
func Equals(t *testing.T, expected any, opts ...ExpectOpt) *Expectation {
	return New(func(arg erltest.ExpectArg) (erltest.Expectation, *erltest.ExpectationFailure) {
		if ok := check.Equal(t, arg.Msg, expected); !ok {
			return nil, Fail(arg, "not equal")
		}
		return nil, nil
	}, opts...)
}

func AttachDeepEqual(tr *erltest.TestReceiver, expected any, cmpOpts []cmp.Option, opts ...ExpectOpt) *Expectation {
	opts = append(opts, Receiver(tr))
	return DeepEqual(tr.T(), expected, cmpOpts, opts...)
}

func DeepEqual(t *testing.T, expected any, cmpOpts []cmp.Option, opts ...ExpectOpt) *Expectation {
	return New(func(arg erltest.ExpectArg) (erltest.Expectation, *erltest.ExpectationFailure) {
		if ok := check.DeepEqual(t, arg.Msg, expected, cmpOpts...); !ok {
			return nil, Fail(arg, "not equal, deeply")
		}
		return nil, nil
	}, opts...)
}
