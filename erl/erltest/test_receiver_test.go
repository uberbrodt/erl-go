package erltest_test

import (
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/erltest"
	"github.com/uberbrodt/erl-go/erl/erltest/expect"
	"github.com/uberbrodt/erl-go/erl/exitreason"
	"github.com/uberbrodt/erl-go/erl/genserver"
)

type testMsg1 struct {
	foo string
}

type testMsg2 struct {
	foo string
	bar string
}

func TestErlTestReceiver_Expectations(t *testing.T) {
	testPID, tr := erltest.NewReceiver(t)

	tr.Expect(testMsg1{}, expect.Expect(func(arg erltest.ExpectArg) bool {
		return true
	}))

	erl.Send(testPID, testMsg1{foo: "bar"})

	tr.Wait()
}

func TestErlTestReceiver_ExpectAbsolute(t *testing.T) {
	testPID, tr := erltest.NewReceiver(t)

	tr.Expect(testMsg2{}, expect.Expect(func(arg erltest.ExpectArg) bool {
		return true
	}, expect.Absolute(4)))

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg2{foo: "bar2"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})

	tr.Wait()
}

func TestErlTestReceiver_FailsIfAbsoluteOutOfOrder(t *testing.T) {
	testPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(time.Second), erltest.NoFail())

	tr.Expect(testMsg2{}, expect.Expect(func(arg erltest.ExpectArg) bool {
		return true
	}, expect.Absolute(4)))

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg2{foo: "bar2"})
	erl.Send(testPID, testMsg1{foo: "bar"})

	tr.Wait()

	f, pass := tr.Pass()

	assert.Assert(t, !pass)
	assert.Assert(t, f == 1)

	assert.Equal(t, tr.Failures()[0].Reason, "expected to match msg #4, but matched with msg #5")
}

func TestReceiver_Expect_AnyTimesPassesIfNoMatch(t *testing.T) {
	_, tr := erltest.NewReceiver(t)

	tr.Expect(testMsg1{}, expect.Expect(func(arg erltest.ExpectArg) bool {
		return true
	}, expect.AnyTimes()))

	tr.Wait()
}

func TestReceiver_Expect_AnyTimesPassesIfMatch(t *testing.T) {
	testPID, tr := erltest.NewReceiver(t)

	foo := make(chan string, 1)

	tr.Expect(testMsg1{}, expect.Expect(func(arg erltest.ExpectArg) bool {
		foo <- "baz"
		return true
	}, expect.AnyTimes()))
	tr.Expect(testMsg2{}, expect.Expect(func(arg erltest.ExpectArg) bool {
		return true
	}))

	erl.Send(testPID, testMsg1{})
	erl.Send(testPID, testMsg2{})

	tr.Wait()
	result := <-foo

	assert.Equal(t, result, "baz")
}

func TestReceiver_Expect_AnyTimePassesImmediatelyWithNoOtherExpectations(t *testing.T) {
	testPID, tr := erltest.NewReceiver(t)

	tr.Expect(testMsg1{}, expect.Expect(func(arg erltest.ExpectArg) bool {
		return true
	}, expect.AnyTimes()))

	erl.Send(testPID, testMsg1{})

	tr.Wait()
}

func TestReceiver_AtMost_PassesIfUnderLimit(t *testing.T) {
	testPID, tr := erltest.NewReceiver(t)

	tr.Expect(testMsg1{}, expect.Expect(func(arg erltest.ExpectArg) bool {
		return true
	}, expect.AtMost(4)))
	tr.Expect(testMsg2{}, expect.Expect(func(arg erltest.ExpectArg) bool {
		return true
	}))

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg2{})

	tr.Wait()
}

func TestReceiver_AtMost_PassesIfAtLimit(t *testing.T) {
	testPID, tr := erltest.NewReceiver(t)

	tr.Expect(testMsg1{}, expect.Expect(func(arg erltest.ExpectArg) bool {
		return true
	}, expect.AtMost(3)))
	tr.Expect(testMsg2{}, expect.Expect(func(arg erltest.ExpectArg) bool {
		return true
	}))

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg2{})

	tr.Wait()
}

func TestReceiver_AtMost_FailsIfOverLimit(t *testing.T) {
	testPID, tr := erltest.NewReceiver(t,
		erltest.WaitTimeout(2*time.Second),
		erltest.ReceiverTimeout(3*time.Second),
		erltest.NoFail())

	tr.Expect(testMsg1{}, expect.Expect(func(erltest.ExpectArg) bool {
		return true
	}, expect.AtMost(3)))
	tr.Expect(testMsg2{}, expect.Expect(func(arg erltest.ExpectArg) bool {
		return true
	}))

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg2{})

	tr.Wait()

	failCnt, pass := tr.Pass()
	assert.Assert(t, failCnt == 1)
	assert.Assert(t, !pass)
	assert.Equal(t, tr.Failures()[0].Reason, "expected to match at most 3 times, but match count is now: 4")
}

func TestReceiver_AtLeast_PassesIfAtTimes(t *testing.T) {
	testPID, tr := erltest.NewReceiver(t)

	tr.Expect(testMsg1{}, expect.Expect(func(arg erltest.ExpectArg) bool {
		return true
	}, expect.AtLeast(2)))

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})

	tr.Wait()
}

func TestReceiver_AtLeast_PassesIfGreaterThanTimes(t *testing.T) {
	testPID, tr := erltest.NewReceiver(t)

	tr.Expect(testMsg1{}, expect.Expect(func(arg erltest.ExpectArg) bool {
		return true
	}, expect.AtLeast(2)))

	tr.Expect(testMsg2{}, expect.Expect(func(arg erltest.ExpectArg) bool {
		return true
	}))

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg2{foo: "bar"})

	tr.Wait()
}

func TestReceiver_AtLeast_FailsIfNotAtTimes(t *testing.T) {
	testPID, tr := erltest.NewReceiver(t,
		erltest.WaitTimeout(time.Second),
		erltest.ReceiverTimeout(2*time.Second),
		erltest.NoFail())

	tr.Expect(testMsg1{}, expect.Expect(func(arg erltest.ExpectArg) bool {
		return true
	}, expect.AtLeast(2)))

	erl.Send(testPID, testMsg1{foo: "bar"})

	f, pass := tr.Pass()
	assert.Assert(t, !pass)
	assert.Assert(t, f == 0)
}

func TestReceiver_AtLeast_MatchesExitMsg(t *testing.T) {
	testPID, tr := erltest.NewReceiver(t,
		erltest.WaitTimeout(time.Second),
		erltest.ReceiverTimeout(2*time.Second),
		erltest.NoFail())

	tr.Expect(erl.ExitMsg{}, expect.Expect(func(arg erltest.ExpectArg) bool {
		return true
	}, expect.AtLeast(2)))

	erl.Exit(testPID, testPID, exitreason.Normal)

	tr.Wait()

	f, pass := tr.Pass()
	assert.Assert(t, !pass)
	assert.Assert(t, f == 0)
}

func TestReceiver_TestExpectationPanick_ReturnsFailure(t *testing.T) {
	testPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(time.Second), erltest.NoFail())

	tr.Expect(erl.ExitMsg{}, expect.Expect(func(arg erltest.ExpectArg) bool {
		panic("whatever man")
	}, expect.AtLeast(2)))

	erl.Exit(testPID, testPID, exitreason.Normal)
	erl.Exit(testPID, testPID, exitreason.Normal)

	tr.Wait()

	f, pass := tr.Pass()
	assert.Assert(t, !pass)
	assert.Assert(t, f > 0)
}

func TestErlTestReceiver_Times_FailsIfLessThan(t *testing.T) {
	testPID, tr := erltest.NewReceiver(t,
		erltest.WaitTimeout(time.Second),
		erltest.ReceiverTimeout(2*time.Second),
		erltest.NoFail())

	tr.Expect(testMsg1{}, expect.Expect(func(arg erltest.ExpectArg) bool {
		return true
	}, expect.Times(4)))

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})

	tr.Wait()

	f, pass := tr.Pass()

	assert.Assert(t, !pass)
	assert.Assert(t, f == 0)
}

func TestErlTestReceiver_Times_FailsIfGreaterThan(t *testing.T) {
	testPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(time.Second*1), erltest.NoFail())

	tr.Expect(testMsg1{}, expect.Called(expect.Times(4)))

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})

	tr.Wait()

	f, pass := tr.Pass()

	assert.Assert(t, !pass)
	assert.Assert(t, f >= 1)
	assert.Equal(t, tr.Failures()[0].Reason, "expected to match 4 times, but match count is now: 5")
}

func TestErlTestReceiver_Times_PassesIfMet(t *testing.T) {
	testPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(time.Second))

	tr.Expect(testMsg1{}, expect.Expect(func(arg erltest.ExpectArg) bool {
		return true
	}, expect.Times(4)))

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})

	tr.Wait()

	f, pass := tr.Pass()

	assert.Assert(t, pass)
	assert.Assert(t, f == 0)
}

func TestErlTestReceiver_TimesZero_PassesIfMet(t *testing.T) {
	testPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(time.Second))

	tr.Expect(testMsg1{}, expect.Expect(func(arg erltest.ExpectArg) bool {
		return true
	}, expect.Times(0)))

	erl.Send(testPID, testMsg2{foo: "bar"})
	erl.Send(testPID, testMsg2{foo: "bar"})

	tr.Wait()

	f, pass := tr.Pass()

	assert.Assert(t, pass)
	assert.Assert(t, f == 0)
}

func TestErlTestReceiver_TimesZero_FailsIfMatched(t *testing.T) {
	testPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(time.Second), erltest.NoFail())

	tr.Expect(testMsg1{}, expect.Expect(func(arg erltest.ExpectArg) bool {
		return true
	}, expect.Times(0)))

	erl.Send(testPID, testMsg2{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})

	tr.Wait()

	f, pass := tr.Pass()

	assert.Assert(t, !pass)
	assert.Assert(t, f == 1)
}

func TestErlTestReceiver_CastExpect_FailsIfNotSatisifed(t *testing.T) {
	testPID, tr := erltest.NewReceiver(t,
		erltest.WaitTimeout(time.Second),
		erltest.ReceiverTimeout(3*time.Second),
		erltest.NoFail())

	tr.ExpectCast(testMsg1{}, expect.Expect(func(arg erltest.ExpectArg) bool {
		return true
	}, expect.Times(3)))

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})

	tr.Wait()

	f, pass := tr.Pass()

	assert.Assert(t, !pass)
	assert.Assert(t, f == 0)
}

func TestErlTestReceiver_CastExpect_PassesIfMatched(t *testing.T) {
	testPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(time.Second))

	tr.ExpectCast(testMsg1{}, expect.Expect(func(arg erltest.ExpectArg) bool {
		return true
	}, expect.Times(3)))

	genserver.Cast(testPID, testMsg1{foo: "bar"})
	genserver.Cast(testPID, testMsg1{foo: "bar"})
	genserver.Cast(testPID, testMsg1{foo: "bar"})

	tr.Wait()

	f, pass := tr.Pass()
	assert.Assert(t, pass)
	assert.Assert(t, f == 0)
}

func TestErlTestReceiver_CallExpect_PassesIfMatched(t *testing.T) {
	self, _ := erltest.NewReceiver(t)
	testPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(2*time.Second))

	tr.ExpectCall(testMsg1{}, expect.Expect(func(arg erltest.ExpectArg) bool {
		t.Logf("Expectation called!")
		genserver.Reply(*arg.From, "baz")
		return true
	}))

	reply, err := genserver.Call(self, testPID, testMsg1{foo: "bar"}, 5*time.Second)
	assert.NilError(t, err)
	assert.Assert(t, reply == "baz")

	tr.Wait()

	f, pass := tr.Pass()
	assert.Assert(t, pass)
	assert.Assert(t, f == 0)
}

func TestErlTestReceiver_CallExpect_FailsIfNotMatched(t *testing.T) {
	self, _ := erltest.NewReceiver(t)
	testPID, tr := erltest.NewReceiver(t,
		erltest.WaitTimeout(time.Second),
		erltest.ReceiverTimeout(2*time.Second),
		erltest.NoFail())

	tr.ExpectCall(testMsg1{}, expect.Expect(func(arg erltest.ExpectArg) bool {
		genserver.Reply(*arg.From, "baz")
		return true
	}, expect.Times(2)))

	reply, err := genserver.Call(self, testPID, testMsg1{foo: "bar"}, 5*time.Second)
	assert.NilError(t, err)
	assert.Assert(t, reply == "baz")

	tr.Wait()

	f, pass := tr.Pass()
	assert.Assert(t, !pass)
	assert.Assert(t, f == 0)
}

func TestErlTestReceiver_NestedExpect(t *testing.T) {
	testPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(time.Second))

	var buzzed bool

	expectBuzz := expect.Expect(func(arg erltest.ExpectArg) bool {
		msg := arg.Msg.(testMsg1)
		if msg.foo == "buzz" {
			buzzed = true
			return true
		}

		return false
	})

	expectFoobar := expect.Expect(func(arg erltest.ExpectArg) bool {
		msg := arg.Msg.(testMsg1)
		if msg.foo == "foobar" {
			buzzed = true
			return true
		}

		return false
	})

	var bazzed bool
	expectBaz := expect.Expect(func(arg erltest.ExpectArg) bool {
		msg := arg.Msg.(testMsg1)
		if msg.foo == "baz" {
			bazzed = true
			return true
		}

		return false
	})

	expectBar := expect.Expect(func(arg erltest.ExpectArg) bool {
		msg := arg.Msg.(testMsg1)
		if msg.foo == "bar" {
			bazzed = true
			return true
		}

		return false
	}, expect.Times(2))

	expect := expect.New(func(arg erltest.ExpectArg) (erltest.Expectation, *erltest.ExpectationFailure) {
		msg := arg.Msg.(testMsg1)
		switch msg.foo {
		case "bar":
			return expectBar, nil
		case "buzz":
			return expectBuzz, nil
		case "baz":
			return expectBaz, nil
		case "foobar":
			return expectFoobar, nil
		}
		return nil, nil
	}, expect.AtMost(5))
	tr.WaitOn(expectBar, expectBaz, expectBuzz, expectFoobar)
	tr.ExpectCast(testMsg1{}, expect)

	genserver.Cast(testPID, testMsg1{foo: "bar"})
	genserver.Cast(testPID, testMsg1{foo: "bar"})
	genserver.Cast(testPID, testMsg1{foo: "buzz"})
	genserver.Cast(testPID, testMsg1{foo: "baz"})
	genserver.Cast(testPID, testMsg1{foo: "foobar"})

	tr.Wait()

	assert.Assert(t, buzzed)
	assert.Assert(t, bazzed)
	assert.Assert(t, expectBar.Satisfied(true))
	assert.Assert(t, expectBaz.Satisfied(true))
	assert.Assert(t, expectBuzz.Satisfied(true))
	assert.Assert(t, expectFoobar.Satisfied(true))
}

func TestErlTestReceiver_ExpectEquals(t *testing.T) {
	testPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(time.Second))

	barMsg := testMsg1{foo: "bar"}
	buzzMsg := testMsg1{foo: "buzz"}
	bazMsg := testMsg1{foo: "baz"}
	foobarMsg := testMsg1{foo: "foobar"}

	expectBuzz := expect.AttachEquals(tr, buzzMsg)
	expectFoobar := expect.AttachEquals(tr, foobarMsg)
	expectBaz := expect.AttachEquals(tr, bazMsg)
	expectBar := expect.AttachEquals(tr, barMsg, expect.Times(2))

	expect := expect.New(func(arg erltest.ExpectArg) (erltest.Expectation, *erltest.ExpectationFailure) {
		msg := arg.Msg.(testMsg1)
		switch msg.foo {
		case "bar":
			return expectBar, nil
		case "buzz":
			return expectBuzz, nil
		case "baz":
			return expectBaz, nil
		case "foobar":
			return expectFoobar, nil
		}
		return nil, nil
	}, expect.AtMost(5))

	tr.ExpectCast(testMsg1{}, expect)

	genserver.Cast(testPID, barMsg)
	genserver.Cast(testPID, barMsg)
	genserver.Cast(testPID, buzzMsg)
	genserver.Cast(testPID, bazMsg)
	genserver.Cast(testPID, foobarMsg)

	tr.Wait()

	assert.Assert(t, expectBar.Satisfied(true))
	assert.Assert(t, expectBaz.Satisfied(true))
	assert.Assert(t, expectBuzz.Satisfied(true))
	assert.Assert(t, expectFoobar.Satisfied(true))
}

func TestErlTestReceiver_ChainedExpect(t *testing.T) {
	testPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(time.Second))

	barMsg := testMsg2{foo: "bar", bar: "baz"}
	buzzMsg := testMsg1{foo: "buzz"}

	chainedExpect := expect.Equals(t, barMsg)

	expectMsg1 := expect.Called(expect.Name("TestMsg1"))
	expectMsg2 := expect.Called(expect.Name("TestMsg2"))
	expectMsg2.And(chainedExpect)

	tr.ExpectCast(testMsg1{}, expectMsg1)
	tr.ExpectCast(testMsg2{}, expectMsg2)

	genserver.Cast(testPID, barMsg)
	genserver.Cast(testPID, buzzMsg)

	tr.Wait()

	assert.Assert(t, chainedExpect.Satisfied(true))
}

func TestErlTestReceiver_CallExpect_ReturnFailure(t *testing.T) {
	testPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(time.Second), erltest.NoFail())
	tr.ExpectCast(testMsg1{}, expect.New(func(arg erltest.ExpectArg) (erltest.Expectation, *erltest.ExpectationFailure) {
		return nil, erltest.Fail(arg, "Didn't like that!")
	}, expect.Name("FooBar Check")))

	genserver.Cast(testPID, testMsg1{foo: "bar"})

	tr.Wait()

	f, pass := tr.Pass()

	assert.Assert(t, !pass)
	assert.Equal(t, f, 1)
	assert.Equal(t, tr.Failures()[0].Exp.Name(), "FooBar Check")
}

func TestReceiver_StopWaitsUntilStopped(t *testing.T) {
	testPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(5*time.Second))

	// sending whatever just to simulate real usage
	for i := 0; i < 5; i++ {
		erl.Send(testPID, testMsg1{})
	}

	tr.Stop()

	t.Logf("checking to see if %+v is still alive", testPID)
	assert.Assert(t, erl.IsAlive(tr.Self()) == false)
	assert.Assert(t, erl.IsAlive(testPID) == false)
}

func TestReceiver_StopKillsDeps(t *testing.T) {
	testPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(5*time.Second))

	pid1 := tr.StartSupervised(func(self erl.PID) (erl.PID, error) {
		pid, _ := erltest.NewReceiver(t)
		return pid, nil
	})

	pid2 := tr.StartSupervised(func(self erl.PID) (erl.PID, error) {
		pid, _ := erltest.NewReceiver(t)
		return pid, nil
	})
	pid3 := tr.StartSupervised(func(self erl.PID) (erl.PID, error) {
		pid, _ := erltest.NewReceiver(t)
		return pid, nil
	})
	pid4 := tr.StartSupervised(func(self erl.PID) (erl.PID, error) {
		pid, _ := erltest.NewReceiver(t)
		return pid, nil
	})

	tr.Stop()

	assert.Assert(t, erl.IsAlive(testPID) == false)
	assert.Assert(t, erl.IsAlive(pid1) == false)
	assert.Assert(t, erl.IsAlive(pid2) == false)
	assert.Assert(t, erl.IsAlive(pid3) == false)
	assert.Assert(t, erl.IsAlive(pid4) == false)
}

func TestReceiver_InfoMsgs_MultipleExpectations(t *testing.T) {
	testPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(5*time.Second))

	tr.Expect(testMsg1{}, expect.Equals(t, testMsg1{foo: "bar"}, expect.AtLeast(2)))
	tr.Expect(testMsg1{}, expect.Equals(t, testMsg1{foo: "foo"}))

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "foo"})

	tr.Wait()

	f, pass := tr.Pass()
	assert.Assert(t, pass)
	assert.Assert(t, f == 0)
}

func TestReceiver_InfoMsgs_AbsoluteExpectationsWillNotBeRemovedUntilAfterWaitFail(t *testing.T) {
	// Setting an absolute expectation in pole position means that
	// it will not be removed until the wait timeout passes
	testPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(5*time.Second), erltest.NoFail())

	tr.Expect(testMsg1{}, expect.Equals(t, testMsg1{foo: "bar"}, expect.Times(2)))
	tr.Expect(testMsg1{}, expect.Equals(t, testMsg1{foo: "foo"}))

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "foo"})

	tr.Wait()

	f, pass := tr.Pass()
	assert.Assert(t, !pass)
	assert.Assert(t, f == 1)
}

func TestReceiver_InfoMsgs_AbsoluteExpectationsWillNotBeRemovedUntilAfterWaitPass(t *testing.T) {
	// Setting an absolute expectation in pole position means that
	// it will not be removed until the wait timeout passes
	testPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(5*time.Second), erltest.NoFail())

	tr.Expect(testMsg1{}, expect.Equals(t, testMsg1{foo: "bar"}, expect.Times(2)))
	tr.Expect(testMsg1{}, expect.Equals(t, testMsg1{foo: "foo"}))

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})

	go func() {
		time.Sleep(time.Second * 6)
		erl.Send(testPID, testMsg1{foo: "foo"})
	}()

	tr.Wait()

	f, pass := tr.Pass()
	assert.Assert(t, pass)
	assert.Assert(t, f == 0)
}

func TestReceiver_CastMsgs_MultipleExpectations(t *testing.T) {
	testPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(5*time.Second))

	tr.ExpectCast(testMsg1{}, expect.Equals(t, testMsg1{foo: "bar"}, expect.AtLeast(2)))
	tr.ExpectCast(testMsg1{}, expect.Equals(t, testMsg1{foo: "foo"}))

	genserver.Cast(testPID, testMsg1{foo: "bar"})
	genserver.Cast(testPID, testMsg1{foo: "bar"})
	genserver.Cast(testPID, testMsg1{foo: "foo"})

	tr.Wait()

	f, pass := tr.Pass()
	assert.Assert(t, pass)
	assert.Assert(t, f == 0)
}

func TestReceiver_CastMsgs_AbsoluteExpectationsWillNotBeRemovedUntilAfterWaitFail(t *testing.T) {
	// Setting an absolute expectation in pole position means that
	// it will not be removed until the wait timeout passes
	testPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(5*time.Second), erltest.NoFail())

	tr.ExpectCast(testMsg1{}, expect.Equals(t, testMsg1{foo: "bar"}, expect.Times(2)))
	tr.ExpectCast(testMsg1{}, expect.Equals(t, testMsg1{foo: "foo"}))

	genserver.Cast(testPID, testMsg1{foo: "bar"})
	genserver.Cast(testPID, testMsg1{foo: "bar"})
	genserver.Cast(testPID, testMsg1{foo: "foo"})

	tr.Wait()

	f, pass := tr.Pass()
	assert.Assert(t, !pass)
	assert.Assert(t, f == 1)
}

func TestReceiver_CastMsgs_AbsoluteExpectationsWillNotBeRemovedUntilAfterWaitPass(t *testing.T) {
	// Setting an absolute expectation in pole position means that
	// it will not be removed until the wait timeout passes
	testPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(5*time.Second), erltest.NoFail())

	tr.ExpectCast(testMsg1{}, expect.Equals(t, testMsg1{foo: "bar"}, expect.Times(2)))
	tr.ExpectCast(testMsg1{}, expect.Equals(t, testMsg1{foo: "foo"}))

	genserver.Cast(testPID, testMsg1{foo: "bar"})
	genserver.Cast(testPID, testMsg1{foo: "bar"})

	go func() {
		time.Sleep(time.Second * 6)
		genserver.Cast(testPID, testMsg1{foo: "foo"})
	}()

	tr.Wait()

	f, pass := tr.Pass()
	assert.Assert(t, pass)
	assert.Assert(t, f == 0)
}
