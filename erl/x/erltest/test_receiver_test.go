package erltest_test

import (
	"testing"
	"time"

	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
	"github.com/uberbrodt/erl-go/erl/genserver"
	"github.com/uberbrodt/erl-go/erl/x/erltest"
	"github.com/uberbrodt/erl-go/erl/x/erltest/internal/mock"
)

type testMsg1 struct {
	foo string
}

type testMsg2 struct {
	foo string
	bar string
}

func TestErlTestReceiver_Expectations(t *testing.T) {
	fakeT := standardFakeT(t, nil)
	testPID, tr := erltest.NewReceiver(fakeT)

	tr.Expect(testMsg1{}, gomock.Eq(testMsg1{foo: "bar"})).MinTimes(1)

	erl.Send(testPID, testMsg1{foo: "bar"})

	tr.Wait()
}

func TestErlTestReceiver_ExpectAbsolute(t *testing.T) {
	fakeT := standardFakeT(t, nil)
	testPID, tr := erltest.NewReceiver(fakeT, erltest.WaitTimeout(time.Second))

	tr.Expect(testMsg2{}, gomock.Any()).Times(1)
	tr.Expect(testMsg1{}, gomock.Any()).Times(5)

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg2{foo: "bar2"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})

	tr.Wait()
	fails := tr.Failures()
	assert.Equal(t, len(fails), 0)
}

func TestErlTestReceiver_FailsIfAbsoluteOutOfOrder(t *testing.T) {
	fakeT := standardFakeT(t, nil)
	testPID, tr := erltest.NewReceiver(fakeT, erltest.WaitTimeout(time.Second))

	first := tr.Expect(testMsg1{}, gomock.Any()).Times(5)
	tr.Expect(testMsg2{}, gomock.Any()).Times(1).After(first)
	// tr.Expect(testMsg2{}, expect.Expect(func(arg erltest.ExpectArg) bool {
	// 	return true
	// }, expect.Absolute(4)))

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg2{foo: "bar2"})
	erl.Send(testPID, testMsg1{foo: "bar"})

	tr.Wait()

	fails := tr.Failures()
	assert.ErrorContains(t, fails[0].Reason, "msg erltest_test.testMsg2 doesn't have a prerequisite expectation satisfied")
}

func TestReceiver_Expect_AnyTimesPassesIfNoMatch(t *testing.T) {
	fakeT := standardFakeT(t, nil)
	_, tr := erltest.NewReceiver(fakeT, erltest.WaitTimeout(time.Second))

	tr.Expect(testMsg1{}, gomock.Any()).AnyTimes()

	tr.Wait()
}

func TestReceiver_Expect_AnyTimesPassesIfMatch(t *testing.T) {
	fakeT := standardFakeT(t, nil)
	testPID, tr := erltest.NewReceiver(fakeT)

	foo := make(chan string, 1)

	tr.Expect(testMsg1{}, gomock.Any()).AnyTimes().Do(func(ea erltest.ExpectArg) {
		foo <- "baz"
	})
	tr.Expect(testMsg2{}, gomock.Any()).AnyTimes()

	erl.Send(testPID, testMsg1{})
	erl.Send(testPID, testMsg2{})

	tr.Wait()
	result := <-foo

	assert.Equal(t, result, "baz")
}

func TestReceiver_Expect_AnyTimePassesImmediatelyWithNoOtherExpectations(t *testing.T) {
	fakeT := standardFakeT(t, nil)
	testPID, tr := erltest.NewReceiver(fakeT)

	tr.Expect(testMsg1{}, gomock.Any()).AnyTimes()
	erl.Send(testPID, testMsg1{})

	tr.Wait()
}

func TestReceiver_AtMost_PassesIfUnderLimit(t *testing.T) {
	fakeT := standardFakeT(t, nil)
	testPID, tr := erltest.NewReceiver(fakeT)

	tr.Expect(testMsg1{}, gomock.Any()).MaxTimes(3)
	tr.Expect(testMsg2{}, gomock.Any()).MaxTimes(2)

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg2{})

	tr.Wait()
}

func TestReceiver_AtMost_PassesIfAtLimit(t *testing.T) {
	fakeT := standardFakeT(t, nil)
	testPID, tr := erltest.NewReceiver(fakeT, erltest.WaitTimeout(time.Second))

	tr.Expect(testMsg1{}, gomock.Any()).MaxTimes(3)
	tr.Expect(testMsg2{}, gomock.Any()).MaxTimes(1)

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg2{})

	tr.Wait()
}

func TestReceiver_AtMost_FailsIfOverLimit(t *testing.T) {
	fakeT := standardFakeT(t, nil)
	testPID, tr := erltest.NewReceiver(fakeT,
		erltest.WaitTimeout(2*time.Second),
		erltest.ReceiverTimeout(3*time.Second),
	)

	tr.Expect(testMsg1{}, gomock.Any()).MaxTimes(3)
	tr.Expect(testMsg2{}, gomock.Any()).MaxTimes(2)

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg2{})

	tr.Wait()

	fails := tr.Failures()
	assert.Equal(t, len(fails), 1)
	assert.ErrorContains(t, fails[0].Reason, "all expectations for msg 'erltest_test.testMsg1{foo:\"bar\"}' have been exhausted")
}

func TestReceiver_AtLeast_PassesIfAtTimes(t *testing.T) {
	fakeT := standardFakeT(t, nil)
	testPID, tr := erltest.NewReceiver(fakeT)

	tr.Expect(testMsg1{}, gomock.Any()).MinTimes(2)

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})

	tr.Wait()
}

func TestReceiver_AtLeast_PassesIfGreaterThanTimes(t *testing.T) {
	fakeT := standardFakeT(t, nil)
	testPID, tr := erltest.NewReceiver(fakeT, erltest.WaitTimeout(1*time.Second))

	tr.Expect(testMsg1{}, gomock.Any()).MinTimes(2)
	tr.Expect(testMsg2{}, gomock.Any())

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg2{foo: "bar"})

	tr.Wait()
}

func TestReceiver_AtLeast_FailsIfNotAtTimes(t *testing.T) {
	fakeT := standardFakeT(t, nil)
	testPID, tr := erltest.NewReceiver(fakeT,
		erltest.WaitTimeout(time.Second),
		erltest.ReceiverTimeout(2*time.Second),
		erltest.NoFail())

	tr.Expect(testMsg1{}, gomock.Any()).MinTimes(2)

	erl.Send(testPID, testMsg1{foo: "bar"})
	tr.Wait()

	fails := tr.Failures()
	assert.Equal(t, len(fails), 1)
	assert.ErrorContains(t, fails[0].Reason, "{erltest_test.testMsg1 matches is anything} MinTimes: 2, MaxTimes: 100000000, CallCount: 1")
}

func TestReceiver_AtLeast_MatchesExitMsg(t *testing.T) {
	fakeT := standardFakeT(t, nil)
	testPID, tr := erltest.NewReceiver(fakeT,
		erltest.WaitTimeout(time.Second), erltest.ReceiverTimeout(2*time.Second),
	)

	ex := tr.Expect(erl.ExitMsg{}, gomock.Any()).AnyTimes().MinTimes(2)

	t.Logf("Min calls: %d", ex.MinCalls())

	erl.Exit(testPID, testPID, exitreason.Normal)

	tr.Wait()
	t.Logf("num calls: %d", ex.CallCount())
	fails := tr.Failures()

	assert.Equal(t, len(fails), 1)
	assert.ErrorContains(t, fails[0].Reason, "{erl.ExitMsg matches is anything} MinTimes: 2, MaxTimes: 100000000, CallCount: 1")
}

func TestReceiver_TestExpectationPanick_FailsTest(t *testing.T) {
	ex := func(ft *mock.MockTLike) {
		ft.EXPECT().Errorf("the Do() Handle for [%s - %s] expectation panicked", gomock.Any(), gomock.Any())
	}
	fakeT := standardFakeT(t, &ex)
	testPID, tr := erltest.NewReceiver(fakeT, erltest.WaitTimeout(time.Second))

	tr.Expect(erl.ExitMsg{}, gomock.Any()).MinTimes(2).Do(func(arg erltest.ExpectArg) {
		panic("whatever man")
	})

	erl.Exit(testPID, testPID, exitreason.Normal)
	erl.Exit(testPID, testPID, exitreason.Normal)

	tr.Wait()
}

func TestErlTestReceiver_Times_FailsIfLessThan(t *testing.T) {
	fakeT := standardFakeT(t, nil)
	testPID, tr := erltest.NewReceiver(fakeT,
		erltest.WaitTimeout(time.Second),
		erltest.ReceiverTimeout(2*time.Second))

	tr.Expect(testMsg1{}, gomock.Any()).Times(4)

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})

	tr.Wait()

	fails := tr.Failures()

	assert.Equal(t, len(fails), 1)
	assert.ErrorContains(t, fails[0].Reason, "{erltest_test.testMsg1 matches is anything} MinTimes: 4, MaxTimes: 4")
}

func TestErlTestReceiver_Times_FailsIfGreaterThan(t *testing.T) {
	fakeT := standardFakeT(t, nil)
	testPID, tr := erltest.NewReceiver(fakeT, erltest.WaitTimeout(time.Second*1))

	tr.Expect(testMsg1{}, gomock.Any()).Times(4)

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})

	tr.Wait()
	fails := tr.Failures()

	assert.Equal(t, len(fails), 2)
	assert.ErrorContains(t, fails[0].Reason, "all expectations for msg 'erltest_test.testMsg1{foo:\"bar\"}' have been exhausted")
	assert.ErrorContains(t, fails[1].Reason, "all expectations for msg 'erltest_test.testMsg1{foo:\"bar\"}' have been exhausted")
}

func TestErlTestReceiver_Times_PassesIfMet(t *testing.T) {
	fakeT := standardFakeT(t, nil)
	testPID, tr := erltest.NewReceiver(fakeT, erltest.WaitTimeout(time.Second))

	tr.Expect(testMsg1{}, gomock.Any()).Times(4)

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})

	tr.Wait()
}

func TestErlTestReceiver_TimesZero_PassesIfMet(t *testing.T) {
	fakeT := standardFakeT(t, nil)
	testPID, tr := erltest.NewReceiver(fakeT, erltest.WaitTimeout(time.Second))

	tr.Expect(testMsg1{}, gomock.Any()).Times(0)

	erl.Send(testPID, testMsg2{foo: "bar"})
	erl.Send(testPID, testMsg2{foo: "bar"})

	tr.Wait()
}

func TestErlTestReceiver_TimesZero_FailsIfMatched(t *testing.T) {
	fakeT := standardFakeT(t, nil)
	testPID, tr := erltest.NewReceiver(fakeT, erltest.WaitTimeout(time.Second))

	tr.Expect(testMsg1{}, gomock.Any()).Times(0)

	erl.Send(testPID, testMsg2{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})

	tr.Wait()

	fails := tr.Failures()

	assert.Equal(t, len(fails), 2)
	assert.ErrorContains(t, fails[0].Reason, "there are no expected calls for msg erltest_test.testMsg2{foo:\"bar\", bar:\"\"}")
	assert.ErrorContains(t, fails[1].Reason, "has already been called the max number of times")
}

func TestErlTestReceiver_CastExpect_FailsIfNotSatisifed(t *testing.T) {
	fakeT := standardFakeT(t, nil)
	testPID, tr := erltest.NewReceiver(fakeT,
		erltest.WaitTimeout(time.Second),
		erltest.ReceiverTimeout(3*time.Second))

	tr.ExpectCast(testMsg1{}, gomock.Eq(testMsg1{foo: "bar"})).Times(4)

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})

	tr.Wait()

	fails := tr.Failures()

	assert.Equal(t, len(fails), 1)
	assert.ErrorContains(t, fails[0].Reason, "{erltest_test.testMsg1 matches is equal to {bar} (erltest_test.testMsg1)} MinTimes: 4, MaxTimes: 4, CallCount:")
}

func TestErlTestReceiver_CastExpect_PassesIfMatched(t *testing.T) {
	fakeT := standardFakeT(t, nil)
	testPID, tr := erltest.NewReceiver(fakeT, erltest.WaitTimeout(time.Second))

	tr.ExpectCast(testMsg1{}, gomock.Any()).Times(3)

	genserver.Cast(testPID, testMsg1{foo: "bar"})
	genserver.Cast(testPID, testMsg1{foo: "bar"})
	genserver.Cast(testPID, testMsg1{foo: "bar"})

	tr.Wait()
}

func TestErlTestReceiver_CallExpect_PassesIfMatched(t *testing.T) {
	fakeT := standardFakeT(t, nil)
	self, _ := erltest.NewReceiver(t)
	testPID, tr := erltest.NewReceiver(fakeT, erltest.WaitTimeout(2*time.Second))

	tr.ExpectCall(testMsg1{}, gomock.Eq(testMsg1{foo: "bar"}), "baz").Times(1)

	reply, err := genserver.Call(self, testPID, testMsg1{foo: "bar"}, 5*time.Second)
	assert.NilError(t, err)
	assert.Assert(t, reply == "baz")

	tr.Wait()
}

func TestErlTestReceiver_CallExpect_FailsIfNotMatched(t *testing.T) {
	fakeT := standardFakeT(t, nil)
	self, _ := erltest.NewReceiver(t)
	testPID, tr := erltest.NewReceiver(fakeT,
		erltest.WaitTimeout(time.Second),
		erltest.ReceiverTimeout(2*time.Second))

	tr.ExpectCall(testMsg1{}, gomock.Eq(testMsg1{foo: "bar"}), erltest.NoCallReply).Times(2).
		Do(func(ea erltest.ExpectArg) {
			genserver.Reply(*ea.From, "baz")
		})

	reply, err := genserver.Call(self, testPID, testMsg1{foo: "bar"}, 5*time.Second)
	assert.NilError(t, err)
	assert.Assert(t, reply == "baz")

	tr.Wait()

	fails := tr.Failures()

	assert.Equal(t, len(fails), 1)
	assert.ErrorContains(t, fails[0].Reason, "{erltest_test.testMsg1 matches is equal to {bar} (erltest_test.testMsg1)} MinTimes: 2, MaxTimes: 2, CallCount: 1")
}

func TestErlTestReceiver_ComplexExpectChains(t *testing.T) {
	fakeT := standardFakeT(t, nil)
	testPID, tr := erltest.NewReceiver(fakeT, erltest.WaitTimeout(time.Second))

	var buzzed bool

	expectBuzz := tr.ExpectCast(testMsg1{}, gomock.Eq(testMsg1{foo: "buzz"})).Times(1).
		Do(func(ea erltest.ExpectArg) {
			buzzed = true
		})

	expectFoobar := tr.ExpectCast(testMsg1{}, gomock.Eq(testMsg1{foo: "foobar"})).Times(1)

	var bazzed bool
	expectBaz := tr.ExpectCast(testMsg1{}, gomock.Eq(testMsg1{foo: "baz"})).Times(1).
		Do(func(ea erltest.ExpectArg) {
			bazzed = true
		})

	expectBar := tr.ExpectCast(testMsg1{}, gomock.Eq(testMsg1{foo: "bar"})).Times(2)

	// tr.WaitOn(expectBar, expectBaz, expectBuzz, expectFoobar)

	expectFoobar.After(expectBaz).After(expectBuzz).After(expectBar)

	genserver.Cast(testPID, testMsg1{foo: "bar"})
	genserver.Cast(testPID, testMsg1{foo: "bar"})
	genserver.Cast(testPID, testMsg1{foo: "buzz"})
	genserver.Cast(testPID, testMsg1{foo: "baz"})
	genserver.Cast(testPID, testMsg1{foo: "foobar"})

	tr.Wait()

	assert.Assert(t, buzzed)
	assert.Assert(t, bazzed)
}

func TestErlTestReceiver_AttachEquals(t *testing.T) {
	t.Skip("skipped because this API is removed")
	// testPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(time.Second))
	//
	// barMsg := testMsg1{foo: "bar"}
	// buzzMsg := testMsg1{foo: "buzz"}
	// bazMsg := testMsg1{foo: "baz"}
	// foobarMsg := testMsg1{foo: "foobar"}
	//
	// expectBuzz := expect.AttachEquals(tr, buzzMsg)
	// expectFoobar := expect.AttachEquals(tr, foobarMsg)
	// expectBaz := expect.AttachEquals(tr, bazMsg)
	// expectBar := expect.AttachEquals(tr, barMsg, expect.Times(2))
	//
	// expect := expect.New(func(arg erltest.ExpectArg) (erltest.Expectation, *erltest.ExpectationFailure) {
	// 	msg := arg.Msg.(testMsg1)
	// 	switch msg.foo {
	// 	case "bar":
	// 		return expectBar, nil
	// 	case "buzz":
	// 		return expectBuzz, nil
	// 	case "baz":
	// 		return expectBaz, nil
	// 	case "foobar":
	// 		return expectFoobar, nil
	// 	}
	// 	return nil, nil
	// }, expect.AtMost(5))
	//
	// tr.ExpectCast(testMsg1{}, expect)
	//
	// genserver.Cast(testPID, barMsg)
	// genserver.Cast(testPID, barMsg)
	// genserver.Cast(testPID, buzzMsg)
	// genserver.Cast(testPID, bazMsg)
	// genserver.Cast(testPID, foobarMsg)
	//
	// tr.Wait()
	//
	// assert.Assert(t, expectBar.Satisfied(true))
	// assert.Assert(t, expectBaz.Satisfied(true))
	// assert.Assert(t, expectBuzz.Satisfied(true))
	// assert.Assert(t, expectFoobar.Satisfied(true))
}

func TestErlTestReceiver_ChainedExpect(t *testing.T) {
	t.Skip("this test is a less thorough ComplexExpectChains")
	// testPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(time.Second))
	//
	// barMsg := testMsg2{foo: "bar", bar: "baz"}
	// buzzMsg := testMsg1{foo: "buzz"}
	//
	// chainedExpect := expect.Equals(t, barMsg)
	//
	// expectMsg1 := expect.Called(expect.Name("TestMsg1"))
	// expectMsg2 := expect.Called(expect.Name("TestMsg2"))
	// expectMsg2.And(chainedExpect)
	//
	// tr.ExpectCast(testMsg1{}, expectMsg1)
	// tr.ExpectCast(testMsg2{}, expectMsg2)
	//
	// genserver.Cast(testPID, barMsg)
	// genserver.Cast(testPID, buzzMsg)
	//
	// tr.Wait()
	//
	// assert.Assert(t, chainedExpect.Satisfied(true))
}

func TestErlTestReceiver_FailTestInsideDoFunc(t *testing.T) {
	exs := func(m *mock.MockTLike) {
		m.EXPECT().Error("DoFailIt").Times(1)
	}
	fakeT := standardFakeT(t, &exs)
	testPID, tr := erltest.NewReceiver(fakeT, erltest.WaitTimeout(time.Second))
	tr.ExpectCast(testMsg1{}, gomock.Eq(testMsg1{foo: "bar"})).Name("FooBar Check").Do(func(ea erltest.ExpectArg) {
		fakeT.Error("DoFailIt")
	}).Times(1)

	genserver.Cast(testPID, testMsg1{foo: "bar"})

	tr.Wait()
}

func TestReceiver_StopWaitsUntilStopped(t *testing.T) {
	testPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(5*time.Second))

	tr.Expect(testMsg1{}, gomock.Any()).Times(5)

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
	fakeT := standardFakeT(t, nil)
	testPID, tr := erltest.NewReceiver(fakeT, erltest.WaitTimeout(1*time.Second))

	first := tr.Expect(testMsg1{}, gomock.Eq(testMsg1{foo: "bar"})).MinTimes(2)
	tr.Expect(testMsg1{}, gomock.Eq(testMsg1{foo: "foo"})).After(first)

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "foo"})

	tr.Wait()
}

func TestReceiver_InfoMsgs_AbsoluteExpectationsWillNotBeRemovedUntilAfterWaitFail(t *testing.T) {
	t.Skip("Not sure this test makes sense anymore. Seems like this is not a behaviour we want to preserve?")
	// Setting an absolute expectation in pole position means that
	// it will not be removed until the wait timeout passes
	fakeT := standardFakeT(t, nil)
	testPID, tr := erltest.NewReceiver(fakeT, erltest.WaitTimeout(5*time.Second))

	first := tr.Expect(testMsg1{}, gomock.Eq(testMsg1{foo: "bar"})).Times(2)
	tr.Expect(testMsg1{}, gomock.Eq(testMsg1{foo: "foo"})).After(first)

	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "foo"})

	tr.Wait()
}

func TestReceiver_InfoMsgs_TimesAndMaxExpectationsWillMatchAfterWaitTimeout(t *testing.T) {
	testPID, tr := erltest.NewReceiver(t,
		erltest.WaitTimeout(3*time.Second), erltest.ReceiverTimeout(10*time.Second))

	tr.Expect(testMsg1{}, gomock.Eq(testMsg1{foo: "bar"})).Times(2)
	tr.Expect(testMsg1{}, gomock.Eq(testMsg1{foo: "foo"})).Times(1)
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})

	go func() {
		time.Sleep(time.Second * 6)
		erl.Send(testPID, testMsg1{foo: "foo"})
	}()

	tr.Wait()
}

func TestReceiver_InfoMsgs_AnyTimesWontBlockAfterWaitTimeout(t *testing.T) {
	testPID, tr := erltest.NewReceiver(t,
		erltest.WaitTimeout(3*time.Second), erltest.ReceiverTimeout(30*time.Second))

	tr.Expect(testMsg1{}, gomock.Eq(testMsg1{foo: "bar"})).Times(2)
	tr.Expect(testMsg1{}, gomock.Eq(testMsg1{foo: "foo"})).AnyTimes()
	erl.Send(testPID, testMsg1{foo: "bar"})
	erl.Send(testPID, testMsg1{foo: "bar"})

	go func() {
		time.Sleep(time.Second * 9)
		erl.Send(testPID, testMsg1{foo: "foo"})
	}()

	beforeWait := time.Now()
	tr.Wait()
	assert.Assert(t, time.Since(beforeWait) < 9*time.Second)
}

func TestReceiver_CastMsgs_MultipleExpectations(t *testing.T) {
	testPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(2*time.Second))

	tr.ExpectCast(testMsg1{}, gomock.Eq(testMsg1{foo: "bar"})).MinTimes(2)
	tr.ExpectCast(testMsg1{}, gomock.Eq(testMsg1{foo: "foo"}))

	genserver.Cast(testPID, testMsg1{foo: "bar"})
	genserver.Cast(testPID, testMsg1{foo: "bar"})
	genserver.Cast(testPID, testMsg1{foo: "foo"})

	tr.Wait()
}

func TestReceiver_CastMsgs_MessagesCanArriveOutOfOrder(t *testing.T) {
	testPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(2*time.Second), erltest.NoFail())

	tr.ExpectCast(testMsg1{}, gomock.Eq(testMsg1{foo: "bar"})).Times(2)
	tr.ExpectCast(testMsg1{}, gomock.Eq(testMsg1{foo: "foo"}))

	genserver.Cast(testPID, testMsg1{foo: "bar"})
	genserver.Cast(testPID, testMsg1{foo: "foo"})
	genserver.Cast(testPID, testMsg1{foo: "bar"})

	tr.Wait()
}

func TestReceiver_CallMsgs_MultipleExpectations(t *testing.T) {
	testPID, tr := erltest.NewReceiver(t, erltest.WaitTimeout(5*time.Second))

	tr.ExpectCall(testMsg1{}, gomock.Eq(testMsg1{foo: "bar"}), nil).MinTimes(2)
	tr.ExpectCall(testMsg1{}, gomock.Eq(testMsg1{foo: "foo"}), nil)

	genserver.Call(erl.RootPID(), testPID, testMsg1{foo: "bar"}, time.Minute)
	genserver.Call(erl.RootPID(), testPID, testMsg1{foo: "bar"}, time.Minute)
	genserver.Call(erl.RootPID(), testPID, testMsg1{foo: "foo"}, time.Minute)

	tr.Wait()
}

func TestReciver_Wait_ReturnsImmediatelyIfNoExpectations(t *testing.T) {
	_, tr := erltest.NewReceiver(t, erltest.WaitTimeout(5*time.Second))
	beforeWait := time.Now()
	tr.Wait()
	assert.Assert(t, time.Since(beforeWait) < 1*time.Second)
}
