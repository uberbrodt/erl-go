package genserver_test

import (
	"errors"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
	"github.com/uberbrodt/erl-go/erl/genserver"
	"github.com/uberbrodt/erl-go/erl/x/erltest"
	"github.com/uberbrodt/erl-go/erl/x/erltest/testcase"
	"github.com/uberbrodt/erl-go/erl/x/erltest/testserver"
)

// These tests verify that Terminate is called according to Erlang GenServer rules:
// 1. When genserver.Stop is called on a process
// 2. When a callback returns an error (stop)
// 3. On panic in a handler
// 4. When receiving an exit signal from parent
// 5. When receiving an exit signal from a linked process (with TrapExit), and HandleInfo
//    returns an error for the ExitMsg

// TerminateCalled is a message sent to the test receiver when Terminate is called.
type TerminateCalled struct {
	Reason error
}

// TestTerminate_CalledOnStop verifies that Terminate is called when genserver.Stop
// is used to stop a GenServer process.
func TestTerminate_CalledOnStop(t *testing.T) {
	tc := testcase.New(t, erltest.WaitTimeout(3*time.Second))

	var serverPID erl.PID
	testPID := tc.TestPID()

	tc.Arrange(func(self erl.PID) {
		// Expect the terminate notification
		tc.Receiver().Expect(TerminateCalled{}, gomock.Any()).Times(1).
			Do(func(ea erltest.ExpectArg) {
				msg := ea.Msg.(TerminateCalled)
				assert.Assert(t, exitreason.IsNormal(msg.Reason),
					"Expected Normal exit reason on Stop, got: %v", msg.Reason)
			})

		// Expect exit message from linked server when it stops
		tc.Receiver().Expect(erl.ExitMsg{}, gomock.Any()).Times(1)

		serverPID = tc.StartServer(func(self erl.PID) (erl.PID, error) {
			return testserver.StartLink(self, testserver.NewConfig().
				SetTerminate(func(srvSelf erl.PID, reason error, state testserver.TestServer) {
					erl.Send(testPID, TerminateCalled{Reason: reason})
				}))
		})
	})

	tc.Act(func() {
		err := genserver.Stop(tc.TestPID(), serverPID)
		assert.NilError(t, err)
	})

	tc.Assert(func() {
		assert.Assert(t, !erl.IsAlive(serverPID))
	})
}

// TestTerminate_CalledOnStopWithCustomReason verifies that the custom exit reason
// is passed to Terminate when using StopReason option.
func TestTerminate_CalledOnStopWithCustomReason(t *testing.T) {
	tc := testcase.New(t, erltest.WaitTimeout(3*time.Second))

	var serverPID erl.PID
	testPID := tc.TestPID()

	tc.Arrange(func(self erl.PID) {
		tc.Receiver().Expect(TerminateCalled{}, gomock.Any()).Times(1).
			Do(func(ea erltest.ExpectArg) {
				msg := ea.Msg.(TerminateCalled)
				assert.Assert(t, exitreason.IsShutdown(msg.Reason),
					"Expected Shutdown exit reason, got: %v", msg.Reason)
				assert.ErrorContains(t, msg.Reason, "custom shutdown")
			})

		// Expect exit message from linked server when it stops
		tc.Receiver().Expect(erl.ExitMsg{}, gomock.Any()).Times(1)

		serverPID = tc.StartServer(func(self erl.PID) (erl.PID, error) {
			return testserver.StartLink(self, testserver.NewConfig().
				SetTerminate(func(srvSelf erl.PID, reason error, state testserver.TestServer) {
					erl.Send(testPID, TerminateCalled{Reason: reason})
				}))
		})
	})

	tc.Act(func() {
		customReason := exitreason.Shutdown(errors.New("custom shutdown")).(*exitreason.S)
		err := genserver.Stop(tc.TestPID(), serverPID, genserver.StopReason(customReason))
		assert.NilError(t, err)
	})

	tc.Assert(func() {
		assert.Assert(t, !erl.IsAlive(serverPID))
	})
}

// CastError is a message that causes the HandleCast to return an error.
type CastError struct {
	Err error
}

// TestTerminate_CalledOnHandleCastError verifies Terminate is called when HandleCast
// returns an error.
func TestTerminate_CalledOnHandleCastError(t *testing.T) {
	tc := testcase.New(t, erltest.WaitTimeout(3*time.Second))

	var serverPID erl.PID
	testPID := tc.TestPID()
	castErr := exitreason.Shutdown(errors.New("cast handler error"))

	tc.Arrange(func(self erl.PID) {
		// Expect terminate notification
		tc.Receiver().Expect(TerminateCalled{}, gomock.Any()).Times(1).
			Do(func(ea erltest.ExpectArg) {
				msg := ea.Msg.(TerminateCalled)
				assert.Assert(t, exitreason.IsShutdown(msg.Reason),
					"Expected Shutdown exit reason, got: %v", msg.Reason)
			})

		// Expect exit message from linked server
		tc.Receiver().Expect(erl.ExitMsg{}, gomock.Any()).Times(1)

		serverPID = tc.StartServer(func(self erl.PID) (erl.PID, error) {
			return testserver.StartLink(self, testserver.NewConfig().
				SetTerminate(func(srvSelf erl.PID, reason error, state testserver.TestServer) {
					erl.Send(testPID, TerminateCalled{Reason: reason})
				}).
				AddCastHandler(CastError{}, func(srvSelf erl.PID, arg any, state testserver.TestServer) (testserver.TestServer, any, error) {
					msg := arg.(CastError)
					return state, nil, msg.Err
				}))
		})
	})

	tc.Act(func() {
		genserver.Cast(serverPID, CastError{Err: castErr})
	})

	tc.Assert(func() {
		assert.Assert(t, !erl.IsAlive(serverPID))
	})
}

// InfoError is a message that causes the HandleInfo to return an error.
type InfoError struct {
	Err error
}

// TestTerminate_CalledOnHandleInfoError verifies Terminate is called when HandleInfo
// returns an error.
func TestTerminate_CalledOnHandleInfoError(t *testing.T) {
	tc := testcase.New(t, erltest.WaitTimeout(3*time.Second))

	var serverPID erl.PID
	testPID := tc.TestPID()
	infoErr := exitreason.Exception(errors.New("info handler error"))

	tc.Arrange(func(self erl.PID) {
		tc.Receiver().Expect(TerminateCalled{}, gomock.Any()).Times(1).
			Do(func(ea erltest.ExpectArg) {
				msg := ea.Msg.(TerminateCalled)
				assert.Assert(t, exitreason.IsException(msg.Reason),
					"Expected Exception exit reason, got: %v", msg.Reason)
			})

		tc.Receiver().Expect(erl.ExitMsg{}, gomock.Any()).Times(1)

		serverPID = tc.StartServer(func(self erl.PID) (erl.PID, error) {
			return testserver.StartLink(self, testserver.NewConfig().
				SetTerminate(func(srvSelf erl.PID, reason error, state testserver.TestServer) {
					erl.Send(testPID, TerminateCalled{Reason: reason})
				}).
				AddInfoHandler(InfoError{}, func(srvSelf erl.PID, arg any, state testserver.TestServer) (testserver.TestServer, any, error) {
					msg := arg.(InfoError)
					return state, nil, msg.Err
				}))
		})
	})

	tc.Act(func() {
		erl.Send(serverPID, InfoError{Err: infoErr})
	})

	tc.Assert(func() {
		assert.Assert(t, !erl.IsAlive(serverPID))
	})
}

// TestTerminate_CalledOnParentExit verifies Terminate is called when the parent
// process exits.
func TestTerminate_CalledOnParentExit(t *testing.T) {
	t.Skip("TODO: need to fix this feature")
	tc := testcase.New(t, erltest.WaitTimeout(3*time.Second))

	var childServerPID erl.PID
	testPID := tc.TestPID()

	tc.Arrange(func(self erl.PID) {
		// Expect terminate notification from the child server
		tc.Receiver().Expect(TerminateCalled{}, gomock.Any()).Times(1).
			Do(func(ea erltest.ExpectArg) {
				msg := ea.Msg.(TerminateCalled)
				assert.Assert(t, exitreason.IsShutdown(msg.Reason),
					"Expected Shutdown exit reason from parent exit, got: %v", msg.Reason)
			})

		// Expect exit message from parent server
		tc.Receiver().Expect(erl.ExitMsg{}, gomock.Any()).Times(1)
	})

	tc.Act(func() {
		// Create a parent server that will start a child
		parentConf := testserver.NewConfig().
			SetInit(func(parentSelf erl.PID, args any) (testserver.TestServer, any, error) {
				// Start the child server as part of parent init
				childPID, err := testserver.StartLink(parentSelf, testserver.NewConfig().
					SetTerminate(func(childSelf erl.PID, reason error, state testserver.TestServer) {
						erl.Send(testPID, TerminateCalled{Reason: reason})
					}))
				if err != nil {
					return testserver.TestServer{}, nil, err
				}
				childServerPID = childPID
				return testserver.TestServer{}, nil, nil
			})

		parentPID, err := testserver.StartLink(tc.TestPID(), parentConf)
		assert.NilError(t, err)

		// Now stop the parent - this should trigger Terminate on the child
		err = genserver.Stop(tc.TestPID(), parentPID, genserver.StopReason(exitreason.Shutdown(errors.New("parent stopping")).(*exitreason.S)))
		assert.NilError(t, err)
	})

	tc.Assert(func() {
		assert.Assert(t, !erl.IsAlive(childServerPID))
	})
}

// PanicCast is a message that causes the HandleCast to panic.
type PanicCast struct {
	Msg string
}

// TestTerminate_CalledOnPanic verifies that Terminate is called when a callback panics.
// Per Erlang behavior, Terminate should be called even on panic.
func TestTerminate_CalledOnPanic(t *testing.T) {
	t.Skip("TODO: need to fix this feature")
	tc := testcase.New(t, erltest.WaitTimeout(3*time.Second))

	var serverPID erl.PID
	testPID := tc.TestPID()

	tc.Arrange(func(self erl.PID) {
		// Terminate SHOULD be called on panic per Erlang rules
		tc.Receiver().Expect(TerminateCalled{}, gomock.Any()).Times(1).
			Do(func(ea erltest.ExpectArg) {
				msg := ea.Msg.(TerminateCalled)
				assert.Assert(t, exitreason.IsException(msg.Reason),
					"Expected Exception exit reason on panic, got: %v", msg.Reason)
			})

		// Expect exit message from linked server
		tc.Receiver().Expect(erl.ExitMsg{}, gomock.Any()).Times(1)

		serverPID = tc.StartServer(func(self erl.PID) (erl.PID, error) {
			return testserver.StartLink(self, testserver.NewConfig().
				SetTerminate(func(srvSelf erl.PID, reason error, state testserver.TestServer) {
					erl.Send(testPID, TerminateCalled{Reason: reason})
				}).
				AddCastHandler(PanicCast{}, func(srvSelf erl.PID, arg any, state testserver.TestServer) (testserver.TestServer, any, error) {
					msg := arg.(PanicCast)
					panic(msg.Msg)
				}))
		})
	})

	tc.Act(func() {
		genserver.Cast(serverPID, PanicCast{Msg: "intentional panic"})
	})

	tc.Assert(func() {
		assert.Assert(t, !erl.IsAlive(serverPID))
	})
}

// TestTerminate_CalledOnLinkedProcessExit_TrapExitTrue tests that when a linked process
// exits, if TrapExit is true and HandleInfo returns an error for the ExitMsg, Terminate is called.
func TestTerminate_CalledOnLinkedProcessExit_TrapExitTrue(t *testing.T) {
	tc := testcase.New(t, erltest.WaitTimeout(3*time.Second))

	var serverPID erl.PID
	testPID := tc.TestPID()

	tc.Arrange(func(self erl.PID) {
		// Expect terminate notification
		tc.Receiver().Expect(TerminateCalled{}, gomock.Any()).Times(1).
			Do(func(ea erltest.ExpectArg) {
				msg := ea.Msg.(TerminateCalled)
				// The reason should be what HandleInfo returned
				assert.Assert(t, exitreason.IsShutdown(msg.Reason),
					"Expected Shutdown from HandleInfo error, got: %v", msg.Reason)
			})

		// Expect exit message from linked server
		tc.Receiver().Expect(erl.ExitMsg{}, gomock.Any()).Times(1)

		serverPID = tc.StartServer(func(self erl.PID) (erl.PID, error) {
			pid, err := testserver.StartLink(self, testserver.NewConfig().
				SetTerminate(func(srvSelf erl.PID, reason error, state testserver.TestServer) {
					erl.Send(testPID, TerminateCalled{Reason: reason})
				}).
				AddInfoHandler(erl.ExitMsg{}, func(srvSelf erl.PID, arg any, state testserver.TestServer) (testserver.TestServer, any, error) {
					// Return an error to trigger Terminate
					return state, nil, exitreason.Shutdown(errors.New("linked process died"))
				}))
			if err != nil {
				return erl.PID{}, err
			}
			// Set TrapExit so we receive ExitMsg instead of dying
			erl.ProcessFlag(pid, erl.TrapExit, true)
			return pid, nil
		})
	})

	tc.Act(func() {
		// Create and link a process, then kill it
		linkedProc := tc.Spawn(&simpleRunnable{
			receive: func(procSelf erl.PID, inbox <-chan any) error {
				<-inbox
				return exitreason.Shutdown(errors.New("linked process exiting"))
			},
		})
		erl.Link(serverPID, linkedProc)

		// Trigger the linked process to exit
		erl.Send(linkedProc, "exit")
	})

	tc.Assert(func() {
		assert.Assert(t, !erl.IsAlive(serverPID))
	})
}

// simpleRunnable is a helper for creating simple test processes.
type simpleRunnable struct {
	receive func(self erl.PID, inbox <-chan any) error
}

func (r *simpleRunnable) Receive(self erl.PID, inbox <-chan any) error {
	return r.receive(self, inbox)
}


