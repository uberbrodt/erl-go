package erltest

import (
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
	"github.com/uberbrodt/erl-go/erl/genserver"
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

	tr.WaitFor(time.Second)
	f, pass := tr.Pass()
	assert.Assert(t, !pass)
	assert.Assert(t, f > 0)
}

func TestErlTestReceiver_Times_FailsIfLessThan(t *testing.T) {
	testPID, tr := NewReceiver(t)
	tr.noFail = true

	tr.Expect(testMsg1{}, func(self erl.PID, anymsg any) bool {
		return true
	}, Times(4))

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})

	tr.WaitFor(time.Second * 1)

	f, pass := tr.Pass()

	assert.Assert(t, !pass)
	assert.Assert(t, f == 0)
}

func TestErlTestReceiver_Times_FailsIfGreaterThan(t *testing.T) {
	testPID, tr := NewReceiver(t)
	tr.noFail = true

	tr.Expect(testMsg1{}, func(self erl.PID, anymsg any) bool {
		return true
	}, Times(4))

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})

	tr.WaitFor(time.Second * 1)

	f, pass := tr.Pass()

	assert.Assert(t, !pass)
	assert.Assert(t, f == 1)
	assert.Equal(t, tr.failures[0].Reason, "expected to match 4 times, but match count is now: 5")
}

func TestErlTestReceiver_Times_PassesIfMet(t *testing.T) {
	testPID, tr := NewReceiver(t)

	tr.Expect(testMsg1{}, func(self erl.PID, anymsg any) bool {
		return true
	}, Times(4))

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})

	tr.WaitFor(time.Second * 1)

	f, pass := tr.Pass()

	assert.Assert(t, pass)
	assert.Assert(t, f == 0)
}

func TestErlTestReceiver_CastExpect_FailsIfNotSatisifed(t *testing.T) {
	testPID, tr := NewReceiver(t)
	tr.noFail = true

	tr.ExpectCast(testMsg1{}, func(self erl.PID, anymsg any) bool {
		return true
	}, Times(3))

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})

	tr.WaitFor(time.Second * 1)

	f, pass := tr.Pass()

	assert.Assert(t, !pass)
	assert.Assert(t, f == 0)
}

func TestErlTestReceiver_CastExpect_PassesIfMatched(t *testing.T) {
	testPID, tr := NewReceiver(t)

	tr.ExpectCast(testMsg1{}, func(self erl.PID, anymsg any) bool {
		return true
	}, Times(3))

	genserver.Cast(testPID, testMsg1{foo: "bar"})
	genserver.Cast(testPID, testMsg1{foo: "bar"})
	genserver.Cast(testPID, testMsg1{foo: "bar"})

	tr.WaitFor(time.Second * 1)

	f, pass := tr.Pass()
	assert.Assert(t, pass)
	assert.Assert(t, f == 0)
}

func TestErlTestReceiver_CallExpect_PassesIfMatched(t *testing.T) {
	self, _ := NewReceiver(t)
	testPID, tr := NewReceiver(t)

	tr.ExpectCall(testMsg1{}, func(self erl.PID, from genserver.From, anymsg any) bool {
		t.Logf("Expectation called!")
		genserver.Reply(from, "baz")
		return true
	})

	reply, err := genserver.Call(self, testPID, testMsg1{foo: "bar"}, 5*time.Second)
	assert.NilError(t, err)
	assert.Assert(t, reply == "baz")

	tr.WaitFor(time.Second * 1)

	f, pass := tr.Pass()
	assert.Assert(t, pass)
	assert.Assert(t, f == 0)
}

func TestErlTestReceiver_CallExpect_FailsIfNotMatched(t *testing.T) {
	self, _ := NewReceiver(t)
	testPID, tr := NewReceiver(t)
	tr.noFail = true

	tr.ExpectCall(testMsg1{}, func(self erl.PID, from genserver.From, anymsg any) bool {
		genserver.Reply(from, "baz")
		return true
	}, Times(2))

	reply, err := genserver.Call(self, testPID, testMsg1{foo: "bar"}, 5*time.Second)
	assert.NilError(t, err)
	assert.Assert(t, reply == "baz")

	tr.WaitFor(time.Second * 1)

	f, pass := tr.Pass()
	assert.Assert(t, !pass)
	assert.Assert(t, f == 0)
}
