package supervisor

import (
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/uberbrodt/erl-go/chronos"
	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
	"github.com/uberbrodt/erl-go/erl/genserver"
)

func TestStopTimeout_ProcessesExitNormally(t *testing.T) {
	trPID, tr := erl.NewTestReceiver(t)

	testMsg1 := genserver.NewTestMsg[TestSrvState](
		genserver.SetContinueProbe[TestSrvState](
			func(self erl.PID, state TestSrvState) (TestSrvState, any, error) {
				erl.Send(trPID, "child1_started")
				return state, nil, nil
			},
		))
	testMsg2 := genserver.NewTestMsg[TestSrvState](
		genserver.SetContinueProbe[TestSrvState](
			func(self erl.PID, state TestSrvState) (TestSrvState, any, error) {
				erl.Send(trPID, "child2_started")
				return state, nil, nil
			},
		))

	sup := TestSup{
		supFlags: NewSupFlags(),
		childSpecs: []ChildSpec{
			NewChildSpec("child1",
				testSrvStartFun[TestSrvState](supChild1, nil,
					genserver.SetTestReceiver[TestSrvState](trPID),
					genserver.SetInitProbe(
						func(self erl.PID, args any) (TestSrvState, any, error) {
							return TestSrvState{}, testMsg1, nil
						},
					))),
			NewChildSpec("child2",
				testSrvStartFun[TestSrvState](supChild2, nil,
					genserver.SetTestReceiver[TestSrvState](trPID),
					genserver.SetInitProbe(
						func(self erl.PID, args any) (TestSrvState, any, error) {
							return TestSrvState{}, testMsg2, nil
						},
					))),
		},
	}

	pid, err := testStartSupervisor(t, sup, nil)

	assert.NilError(t, err)
	assert.Assert(t, erl.IsAlive(pid))

	var child1Started bool
	var child2Started bool
	tr.Loop(func(anyMsg any) bool {
		switch msg := anyMsg.(type) {
		case string:
			if msg == "child1_started" {
				child1Started = true
			}
			if msg == "child2_started" {
				child2Started = true
			}
		default:
			// ignore
		}

		return child1Started && child2Started
	})

	// making sure this comes from a parent
	err = genserver.Stop(erl.RootPID(), pid, genserver.StopReason(exitreason.SupervisorShutdown), genserver.StopTimeout(chronos.Dur("60s")))

	assert.NilError(t, err)
}

func TestStopTimeout_ReturnsErrorWhenChildTimesout(t *testing.T) {
	trPID, tr := erl.NewTestReceiver(t)

	testMsg1 := genserver.NewTestMsg[TestSrvState](
		genserver.SetContinueProbe[TestSrvState](
			func(self erl.PID, state TestSrvState) (TestSrvState, any, error) {
				erl.Send(trPID, "child1_started")
				return state, nil, nil
			},
		))

	testMsg2 := genserver.NewTestMsg[TestSrvState](
		genserver.SetContinueProbe[TestSrvState](
			func(self erl.PID, state TestSrvState) (TestSrvState, any, error) {
				erl.Send(trPID, "child2_started")
				return state, nil, nil
			},
		))

	sup := TestSup{
		supFlags: NewSupFlags(),
		childSpecs: []ChildSpec{
			NewChildSpec("child1",
				testSrvStartFun[TestSrvState](supChild1, nil,
					genserver.SetTestReceiver[TestSrvState](trPID),
					genserver.SetInitProbe(
						func(self erl.PID, args any) (TestSrvState, any, error) {
							return TestSrvState{}, testMsg1, nil
						},
					))),
			NewChildSpec("child2",
				testSrvStartFun[TestSrvState](supChild2, nil,
					genserver.SetTestReceiver[TestSrvState](trPID),
					genserver.SetInitProbe(
						func(self erl.PID, args any) (TestSrvState, any, error) {
							erl.ProcessFlag(self, erl.TrapExit, true)
							return TestSrvState{}, testMsg2, nil
						},
					),
					genserver.SetTermProbe(func(self erl.PID, reason error, state TestSrvState) {
						<-time.After(chronos.Dur("5s"))
					}),
				), SetShutdown(ShutdownOpt{Timeout: 2})),
		},
	}

	pid, err := testStartSupervisor(t, sup, nil)

	assert.NilError(t, err)
	assert.Assert(t, erl.IsAlive(pid))

	var child1Started bool
	var child2Started bool
	tr.Loop(func(anyMsg any) bool {
		switch msg := anyMsg.(type) {
		case string:
			if msg == "child1_started" {
				child1Started = true
			}
			if msg == "child2_started" {
				child2Started = true
			}
		default:
			// ignore
		}

		return child1Started && child2Started
	})

	// making sure this comes from a parent
	err = genserver.Stop(erl.RootPID(), pid, genserver.StopReason(exitreason.SupervisorShutdown), genserver.StopTimeout(chronos.Dur("60s")))

	assert.NilError(t, err)
}
