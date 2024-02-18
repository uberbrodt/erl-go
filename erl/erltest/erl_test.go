// tests our
package erltest

import (
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
)

type testMsg1 struct {
	foo string
}

type testMsg2 struct {
	foo string
}

func TestErlTestReceiver_Expectations(t *testing.T) {
	testPID, tr := NewReceiver(t)

	tr.Expect(testMsg1{}, func(self erl.PID, anymsg any) bool {
		return true
	})

	erl.Send(testPID, testMsg1{foo: "bar"})

	tr.Wait()
}

func TestErlTestReceiver_ExpectAbsolute(t *testing.T) {
	testPID, tr := NewReceiver(t)

	tr.Expect(testMsg2{}, func(self erl.PID, anymsg any) bool {
		return true
	}, Absolute(4))

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg2{foo: "bar2"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})

	tr.Wait()
}

func TestErlTestReceiver_FailsIfAbsoluteOutOfOrder(t *testing.T) {
	testPID, tr := NewReceiver(t)
	tr.noFail = true

	tr.Expect(testMsg2{}, func(self erl.PID, anymsg any) bool {
		return true
	}, Absolute(4))

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg2{foo: "bar2"})
	erl.Send(testPID, testMsg1{foo: "bar"})

	tr.WaitFor(time.Second * 1)

	f, pass := tr.Pass()

	assert.Assert(t, !pass)
	assert.Assert(t, f == 1)

	assert.Equal(t, tr.failures[0].Reason, "expected to match msg #4, but matched with msg #5")
}

func TestReceiver_Expect_AnyTimesPassesIfNoMatch(t *testing.T) {
	_, tr := NewReceiver(t)

	tr.Expect(testMsg1{}, func(self erl.PID, anymsg any) bool {
		return true
	}, AnyTimes())

	tr.Wait()
}

func TestReceiver_Expect_AnyTimesPassesIfMatch(t *testing.T) {
	testPID, tr := NewReceiver(t)

	foo := "bar"

	tr.Expect(testMsg1{}, func(self erl.PID, anymsg any) bool {
		foo = "baz"
		return true
	}, AnyTimes())
	tr.Expect(testMsg2{}, func(self erl.PID, anymsg any) bool {
		return true
	})

	erl.Send(testPID, testMsg1{})
	erl.Send(testPID, testMsg2{})

	tr.Wait()

	assert.Equal(t, foo, "baz")
}

func TestReceiver_Expect_AnyTimePassesImmediatelyWithNoOtherExpectations(t *testing.T) {
	testPID, tr := NewReceiver(t)

	tr.Expect(testMsg1{}, func(self erl.PID, anymsg any) bool {
		return true
	}, AnyTimes())

	erl.Send(testPID, testMsg1{})

	tr.Wait()
}

func TestReceiver_AtMost_PassesIfUnderLimit(t *testing.T) {
	testPID, tr := NewReceiver(t)

	tr.Expect(testMsg1{}, func(self erl.PID, anymsg any) bool {
		return true
	}, AtMost(4))
	tr.Expect(testMsg2{}, func(self erl.PID, anymsg any) bool {
		return true
	})

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg2{})

	tr.Wait()
}

func TestReceiver_AtMost_PassesIfAtLimit(t *testing.T) {
	testPID, tr := NewReceiver(t)

	tr.Expect(testMsg1{}, func(self erl.PID, anymsg any) bool {
		return true
	}, AtMost(3))
	tr.Expect(testMsg2{}, func(self erl.PID, anymsg any) bool {
		return true
	})

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg2{})

	tr.Wait()
}

func TestReceiver_AtMost_FailsIfOverLimit(t *testing.T) {
	testPID, tr := NewReceiver(t)
	tr.noFail = true

	tr.Expect(testMsg1{}, func(self erl.PID, anymsg any) bool {
		return true
	}, AtMost(3))
	tr.Expect(testMsg2{}, func(self erl.PID, anymsg any) bool {
		return true
	})

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg2{})

	tr.Wait()

	failCnt, pass := tr.Pass()
	assert.Assert(t, failCnt == 1)
	assert.Assert(t, pass)
	assert.Equal(t, tr.failures[0].Reason, "expected to match at most 3 times, but match count is now: 4")
}

func TestReceiver_AtLeast_PassesIfAtTimes(t *testing.T) {
	testPID, tr := NewReceiver(t)

	tr.Expect(testMsg1{}, func(self erl.PID, anymsg any) bool {
		return true
	}, AtLeast(2))

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})

	tr.Wait()
}

func TestReceiver_AtLeast_PassesIfGreaterThanTimes(t *testing.T) {
	testPID, tr := NewReceiver(t)

	tr.Expect(testMsg1{}, func(self erl.PID, anymsg any) bool {
		return true
	}, AtLeast(2))

	tr.Expect(testMsg2{}, func(self erl.PID, anymsg any) bool {
		return true
	})

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg2{foo: "bar"})

	tr.Wait()
}

func TestReceiver_AtLeast_FailsIfNotAtTimes(t *testing.T) {
	testPID, tr := NewReceiver(t)
	tr.noFail = true

	tr.Expect(testMsg1{}, func(self erl.PID, anymsg any) bool {
		return true
	}, AtLeast(2))

	erl.Send(testPID, testMsg1{foo: "bar"})

	tr.WaitFor(time.Second)

	f, pass := tr.Pass()
	assert.Assert(t, !pass)
	assert.Assert(t, f == 0)
}

func TestReceiver_AtLeast_MatchesExitMsg(t *testing.T) {
	testPID, tr := NewReceiver(t)
	tr.noFail = true

	tr.Expect(erl.ExitMsg{}, func(self erl.PID, anymsg any) bool {
		return true
	}, AtLeast(2))

	erl.Exit(testPID, testPID, exitreason.Normal)
	// erl.Send(testPID, testMsg1{foo: "bar"})

	tr.WaitFor(time.Second)

	f, pass := tr.Pass()
	assert.Assert(t, !pass)
	assert.Assert(t, f == 0)
}

func TestReceiver_TestExpectationPanick_ReturnsFailure(t *testing.T) {
	testPID, tr := NewReceiver(t)
	tr.noFail = true

	tr.Expect(erl.ExitMsg{}, func(self erl.PID, anymsg any) bool {
		panic("whatever man")
	}, AtLeast(2))

	erl.Exit(testPID, testPID, exitreason.Normal)
	erl.Exit(testPID, testPID, exitreason.Normal)
	// erl.Send(testPID, testMsg1{foo: "bar"})

	tr.WaitFor(time.Second)
	f, pass := tr.Pass()
	assert.Assert(t, !pass)
	assert.Assert(t, f > 0)
}