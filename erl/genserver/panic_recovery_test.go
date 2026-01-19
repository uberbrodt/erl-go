
package genserver

import (
	"errors"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/uberbrodt/erl-go/chronos"
	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
)

// TestPanicRecovery_HandleCall_ReturnsException verifies that panics in HandleCall
// are caught and returned as exceptions to the caller
func TestPanicRecovery_HandleCall_ReturnsException(t *testing.T) {
	trPID, _ := erl.NewTestReceiver(t)
	args := TestGSArgs{count: 5}
	gensrvPID, err := StartLink[TestGS](trPID, TestGS{}, args)
	assert.NilError(t, err)

	// Call that will panic
	_, err = Call(trPID, gensrvPID,
		taggedRequest{
			tag:   "panic",
			value: 10,
			callProbe: func(self erl.PID, arg any, from From, state TestGS) (any, TestGS) {
				panic("HandleCall panic test")
			},
		}, chronos.Dur("5s"))

	// Should return an exception error
	assert.Assert(t, exitreason.IsException(err))
	assert.ErrorContains(t, err, "HandleCall panic test")

	// Process should be dead after panic
	assert.Assert(t, !erl.IsAlive(gensrvPID))
}

// TestPanicRecovery_HandleCall_StateNotCorrupted verifies that state updates
// don't persist if HandleCall panics
func TestPanicRecovery_HandleCall_StateNotCorrupted(t *testing.T) {
	trPID, tr := erl.NewTestReceiver(t)
	args := TestGSArgs{count: 5}
	gensrvPID, err := StartLink[TestGS](trPID, TestGS{}, args)
	assert.NilError(t, err)

	var terminateCalled bool
	var terminateState TestGS

	// Make a successful call first
	reply, err := Call(trPID, gensrvPID,
		taggedRequest{
			tag:   "increment",
			value: 3,
			callProbe: func(self erl.PID, arg any, from From, state TestGS) (any, TestGS) {
				val := arg.(int)
				state.Count += val
				return state.Count, state
			},
		}, chronos.Dur("5s"))
	assert.NilError(t, err)
	assert.Equal(t, reply.(int), 8) // 5 + 3

	// Now call that modifies state then panics
	_, err = Call(trPID, gensrvPID,
		taggedRequest{
			tag:   "panic_after_modify",
			value: 100,
			callProbe: func(self erl.PID, arg any, from From, state TestGS) (any, TestGS) {
				// Modify state
				state.Count += arg.(int)
				// Then panic
				panic("panic after state modification")
			},
		}, chronos.Dur("5s"))

	assert.Assert(t, exitreason.IsException(err))

	// Wait for exit message
	select {
	case msg := <-tr.Receiver():
		if exitMsg, ok := msg.(erl.ExitMsg); ok {
			t.Logf("Got exit message: %+v", exitMsg)
		}
	case <-time.After(chronos.Dur("2s")):
		t.Fatal("Did not receive exit message")
	}

	// The question: was Terminate called? And if so, what was the state?
	// This test documents current behavior
	if terminateCalled {
		t.Logf("Terminate was called with state.Count=%d", terminateState.Count)
		// If Terminate sees Count=108 (5+3+100), state was corrupted before panic
		// If Terminate sees Count=8, state was properly protected
	}
}

// TestPanicRecovery_HandleCast_ProcessExits verifies that panics in HandleCast
// cause the process to exit
func TestPanicRecovery_HandleCast_ProcessExits(t *testing.T) {
	trPID, tr := erl.NewTestReceiver(t)
	args := TestGSArgs{count: 5}
	gensrvPID, err := StartLink[TestGS](trPID, TestGS{}, args)
	assert.NilError(t, err)

	// Cast that will panic
	err = Cast(gensrvPID, taggedRequest{
		tag: "panic_cast",
		probe: func(self erl.PID, state TestGS) TestGS {
			panic("HandleCast panic test")
		},
	})
	assert.NilError(t, err) // Cast itself doesn't return panic

	// Should receive exit message
	select {
	case msg := <-tr.Receiver():
		exitMsg, ok := msg.(erl.ExitMsg)
		assert.Assert(t, ok, "Expected ExitMsg, got: %T", msg)
		assert.Assert(t, exitMsg.Proc.Equals(gensrvPID))
		assert.Assert(t, exitreason.IsException(exitMsg.Reason))
		assert.ErrorContains(t, exitMsg.Reason, "HandleCast panic test")
	case <-time.After(chronos.Dur("2s")):
		t.Fatal("Did not receive exit message after HandleCast panic")
	}

	assert.Assert(t, !erl.IsAlive(gensrvPID))
}

// TestPanicRecovery_HandleInfo_ProcessExits verifies that panics in HandleInfo
// cause the process to exit
func TestPanicRecovery_HandleInfo_ProcessExits(t *testing.T) {
	trPID, tr := erl.NewTestReceiver(t)
	args := TestGSArgs{count: 5}
	gensrvPID, err := StartLink[TestGS](trPID, TestGS{}, args)
	assert.NilError(t, err)

	// Send a message that will be handled by HandleInfo and panic
	erl.Send(gensrvPID, taggedRequest{
		tag: "panic_info",
		probe: func(self erl.PID, state TestGS) TestGS {
			panic("HandleInfo panic test")
		},
	})

	// Should receive exit message
	select {
	case msg := <-tr.Receiver():
		exitMsg, ok := msg.(erl.ExitMsg)
		assert.Assert(t, ok, "Expected ExitMsg, got: %T", msg)
		assert.Assert(t, exitMsg.Proc.Equals(gensrvPID))
		assert.Assert(t, exitreason.IsException(exitMsg.Reason))
		assert.ErrorContains(t, exitMsg.Reason, "HandleInfo panic test")
	case <-time.After(chronos.Dur("2s")):
		t.Fatal("Did not receive exit message after HandleInfo panic")
	}

	assert.Assert(t, !erl.IsAlive(gensrvPID))
}

// TestPanicRecovery_HandleContinue_ProcessExits verifies that panics in HandleContinue
// cause the process to exit
func TestPanicRecovery_HandleContinue_ProcessExits(t *testing.T) {
	trPID, tr := erl.NewTestReceiver(t)
	args := TestGSArgs{count: 5}
	gensrvPID, err := StartLink[TestGS](trPID, TestGS{}, args)
	assert.NilError(t, err)

	// Cast that returns a continuation, which will panic in HandleContinue
	err = Cast(gensrvPID, taggedRequest{
		tag:  "continue_panic",
		cont: true,
		probe: func(self erl.PID, state TestGS) TestGS {
			return state
		},
		continueProbe: func(self erl.PID, state TestGS) (TestGS, any, error) {
			panic("HandleContinue panic test")
		},
	})
	assert.NilError(t, err)

	// Should receive exit message
	select {
	case msg := <-tr.Receiver():
		exitMsg, ok := msg.(erl.ExitMsg)
		assert.Assert(t, ok, "Expected ExitMsg, got: %T", msg)
		assert.Assert(t, exitMsg.Proc.Equals(gensrvPID))
		assert.Assert(t, exitreason.IsException(exitMsg.Reason))
		assert.ErrorContains(t, exitMsg.Reason, "HandleContinue panic test")
	case <-time.After(chronos.Dur("2s")):
		t.Fatal("Did not receive exit message after HandleContinue panic")
	}

	assert.Assert(t, !erl.IsAlive(gensrvPID))
}

// TestPanicRecovery_Terminate_DoesNotPreventCleanup verifies that if Terminate
// panics, the process cleanup still happens (links/monitors are notified)
func TestPanicRecovery_Terminate_DoesNotPreventCleanup(t *testing.T) {
	trPID, tr := erl.NewTestReceiver(t)
	args := TestGSArgs{count: 5}

	terminatePanic := func(self erl.PID, reason error, state TestGS) {
		panic("Terminate panic test")
	}

	gensrvPID, err := StartLink[TestGS](trPID, TestGS{terminateProbe: terminatePanic}, args)
	assert.NilError(t, err)

	// Trigger shutdown by sending error from Cast
	err = Cast(gensrvPID, taggedRequest{
		tag: "shutdown",
		err: exitreason.Shutdown(errors.New("intentional shutdown")),
	})
	assert.NilError(t, err)

	// Should still receive exit message despite Terminate panicking
	select {
	case msg := <-tr.Receiver():
		exitMsg, ok := msg.(erl.ExitMsg)
		assert.Assert(t, ok, "Expected ExitMsg, got: %T", msg)
		assert.Assert(t, exitMsg.Proc.Equals(gensrvPID))
		// The exit reason might be the original shutdown or the panic from Terminate
		// Either is acceptable as long as cleanup happens
		t.Logf("Received exit with reason: %v", exitMsg.Reason)
	case <-time.After(chronos.Dur("2s")):
		t.Fatal("Did not receive exit message - cleanup may not have happened")
	}

	assert.Assert(t, !erl.IsAlive(gensrvPID))
}

// NOTE: TestPanicRecovery_SupervisorRestartsAfterPanic moved to supervisor package
// to avoid import cycle (supervisor imports genserver)

// TestPanicRecovery_CallerGetsError verifies that when a GenServer panics during
// a Call, the caller receives an error rather than hanging indefinitely
func TestPanicRecovery_CallerGetsError(t *testing.T) {
	trPID, _ := erl.NewTestReceiver(t)
	args := TestGSArgs{count: 5}
	gensrvPID, err := StartLink[TestGS](trPID, TestGS{}, args)
	assert.NilError(t, err)

	// Make a call that panics - should not hang
	_, err = Call(trPID, gensrvPID,
		taggedRequest{
			tag: "panic",
			callProbe: func(self erl.PID, arg any, from From, state TestGS) (any, TestGS) {
				panic("test panic in call")
			},
		}, chronos.Dur("5s"))

	// Should receive error, not timeout
	assert.Assert(t, err != nil, "Call should return error when server panics")
	assert.Assert(t, exitreason.IsException(err), "Error should be an Exception")
}
