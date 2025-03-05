package erltest

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/rs/xid"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/genserver"
)

type ExpectArg struct {
	Msg any
	// This is only populated for Calls
	From *genserver.From
	// test receiver pid
	Self erl.PID
	Exp  *Expectation
}

// A function that can be used as a  [Expectation.Do]
type DoFun func(ExpectArg)

// A TestReporter is something that can be used to report test failures.  It
// is satisfied by the standard library's *testing.T.
type TestReporter interface {
	// logs an error and fails the test
	Errorf(format string, args ...any)
	// logs an error and panics the test. Be careful with this one, panicking outside
	// the thread where the test started is an error
	Fatalf(format string, args ...any)
	// logs that will show up only if a test fails
	Logf(format string, args ...any)
}

// TestHelper is a TestReporter that has the Helper method.  It is satisfied
// by the standard library's *testing.T.
type TestHelper interface {
	TestReporter
	Helper()
}

type Expectation struct {
	msgT     reflect.Type
	do       DoFun
	minCalls int
	maxCalls int
	numCalls int
	id       string
	name     string
	t        TestHelper
	preReqs  []*Expectation // prerequisite calls
	reply    any
	matcher  Matcher
	mx       sync.Mutex
}

func (e *Expectation) MaxCalls() int {
	return e.maxCalls
}

func (e *Expectation) MinCalls() int {
	return e.minCalls
}

func (e *Expectation) CallCount() int {
	return e.numCalls
}

func newExpect(t TestHelper, m Matcher, matchTerm reflect.Type) *Expectation {
	return &Expectation{t: t, matcher: m, msgT: matchTerm, id: xid.New().String(), maxCalls: 1}
}

func (e *Expectation) Match(msg any) error {
	if !e.matcher.Matches(msg) {
		return fmt.Errorf(
			"expectation doesn't match the msg \nGot: %v\nWant: %v",
			formatGottenArg(e.matcher, msg), e.matcher,
		)
	}

	// Check that all prerequisite calls have been satisfied.
	for _, preReqCall := range e.preReqs {
		if !preReqCall.satisfied() {
			return fmt.Errorf("msg %T doesn't have a prerequisite expectation satisfied:\n%v\nshould be called before:\n%v",
				msg, preReqCall, e)
		}
	}

	// Check that the call is not exhausted.
	if e.exhausted() {
		return fmt.Errorf("expected %+v has already been called the max number of times", e)
	}
	return nil
}

func (e *Expectation) call() DoFun {
	e.mx.Lock()
	defer e.mx.Unlock()
	e.numCalls++
	return e.do
}

// AnyTimes allows the expectation to be called 0 or more times
func (e *Expectation) AnyTimes() *Expectation {
	e.minCalls, e.maxCalls = 0, 1e8 // close enough to infinity
	return e
}

// MinTimes requires the call to occur at least n times. If AnyTimes or MaxTimes have not been called or if MaxTimes
// was previously called with 1, MinTimes also sets the maximum number of calls to infinity.
func (e *Expectation) MinTimes(n int) *Expectation {
	e.minCalls = n
	if e.maxCalls == 1 {
		e.maxCalls = 1e8
	}
	return e
}

// MaxTimes limits the number of calls to n times. If AnyTimes or MinTimes have not been called or if MinTimes was
// previously called with 1, MaxTimes also sets the minimum number of calls to 0.
func (e *Expectation) MaxTimes(n int) *Expectation {
	e.maxCalls = n
	if e.minCalls == 1 {
		e.minCalls = 0
	}
	return e
}

// Times declares the exact number of times a function call is expected to be executed.
func (e *Expectation) Times(n int) *Expectation {
	e.minCalls, e.maxCalls = n, n
	return e
}

// sets the name for the [Expectation], which will be used in error msgs
func (e *Expectation) Name(name string) *Expectation {
	e.name = name
	return e
}

// Sets the Reply that will be sent in the event that this [Expectation]
// matches a Call message. It will not be used if it matches a Cast
func (e *Expectation) Reply(reply any) *Expectation {
	e.reply = reply
	return e
}

// After declares that the call may only match after preReq has been exhausted.
func (e *Expectation) After(preReq *Expectation) *Expectation {
	e.t.Helper()

	if e == preReq {
		e.t.Fatalf("A call isn't allowed to be its own prerequisite")
	}
	if preReq.isPreReq(e) {
		e.t.Fatalf("Loop in call order: %v is a prerequisite to %v (possibly indirectly).", e, preReq)
	}

	e.preReqs = append(e.preReqs, preReq)
	return e
}

// Do declares the action to run when the call is matched. The function will
// receive an [ExpectArg] after the expectation is matched. If [f] panicks it
// will fail the test.
func (e *Expectation) Do(f DoFun) *Expectation {
	e.do = func(arg ExpectArg) {
		defer func() {
			if r := recover(); r != nil {
				e.t.Errorf("the Do() Handle for [%s - %s] expectation panicked", e.id, e.name)
			}
		}()

		e.t.Helper()
		f(arg)
	}
	return e
}

// returns true if the expectation has a max
// number of calls it's expecting.
func (e *Expectation) MustWait() bool {
	return e.maxCalls < 1e8
}

// Reports whether the minimum number of calls have been made.
func (e *Expectation) satisfied() bool {
	e.mx.Lock()
	defer e.mx.Unlock()
	return e.numCalls >= e.minCalls
}

// Reports whether the maximum number of calls have been made.
func (e *Expectation) exhausted() bool {
	e.mx.Lock()
	defer e.mx.Unlock()
	return e.numCalls >= e.maxCalls
}

// dropPrereqs tells the Expectation to not re-check prerequisite calls any
// longer, and to return its current set.
func (e *Expectation) dropPrereqs() (preReqs []*Expectation) {
	e.mx.Lock()
	defer e.mx.Unlock()
	preReqs = e.preReqs
	e.preReqs = nil
	return
}

// isPreReq returns true if other is a direct or indirect prerequisite to c.
func (e *Expectation) isPreReq(other *Expectation) bool {
	e.mx.Lock()
	defer e.mx.Unlock()
	for _, preReq := range e.preReqs {
		if other == preReq || preReq.isPreReq(other) {
			return true
		}
	}
	return false
}

func (e *Expectation) String() string {
	return fmt.Sprintf("erltest.Expectation{id: %s, name: %s}{%s matches %v} MinTimes: %d, MaxTimes: %d, CallCount: %d", e.id, e.name, e.msgT, e.matcher, e.minCalls, e.maxCalls, e.numCalls)
}

type ExpectationFailure struct {
	// The matching term that led to the ExpectationFailure
	Match any
	// The failed expectation
	Exp *Expectation
	// The actual message that matched [Match] and was evaluated by the Expectation
	Msg any
	// Failure message
	Reason error
}

func (ef *ExpectationFailure) String() string {
	return fmt.Sprintf("%v", ef.Reason)
}
