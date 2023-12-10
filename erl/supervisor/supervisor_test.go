package supervisor

import (
	"errors"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/uberbrodt/erl-go/chronos"
	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
	"github.com/uberbrodt/erl-go/erl/genserver"
)

const (
	supChild1 erl.Name = "sup-child-1"
	supChild2 erl.Name = "sup-child-2"
	supChild3 erl.Name = "sup-child-3"
	supChild4 erl.Name = "sup-child-4"
	supName   erl.Name = "test-supervisor"
)

func testSrvStartFun[STATE any](name erl.Name, arg any, opts ...func(ts genserver.TestServer[STATE]) genserver.TestServer[STATE]) func(sup erl.PID) (erl.PID, error) {
	ts := genserver.NewTestServer[STATE](opts...)
	return func(sup erl.PID) (erl.PID, error) {
		return genserver.StartLink[STATE](
			sup,
			ts,
			arg,
			genserver.SetName(name),
		)
	}
}

func TestStartLink_StartsChildren(t *testing.T) {
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

	child1PID, child1Exists := erl.WhereIs(supChild1)
	assert.Assert(t, child1Exists)
	assert.Assert(t, erl.IsAlive(child1PID))

	child2PID, child2Exists := erl.WhereIs(supChild2)
	assert.Assert(t, child2Exists)
	assert.Assert(t, erl.IsAlive(child2PID))
}

func TestStartLink_HandlesIgnoredChildren(t *testing.T) {
	sup := TestSup{
		supFlags: NewSupFlags(),
		childSpecs: []ChildSpec{
			NewChildSpec("child1",
				testSrvStartFun[TestSrvState](supChild1, nil)),
			NewChildSpec("child2",
				testSrvStartFun[TestSrvState](supChild2, nil,
					genserver.SetInitProbe(
						func(self erl.PID, args any) (TestSrvState, any, error) {
							return TestSrvState{}, nil, exitreason.Ignore
						},
					))),
		},
	}
	pid, err := testStartSupervisor(t, sup, nil)

	assert.NilError(t, err)
	assert.Assert(t, erl.IsAlive(pid))

	child1PID, child1Exists := erl.WhereIs(supChild1)
	assert.Assert(t, child1Exists)
	assert.Assert(t, erl.IsAlive(child1PID))

	child2PID, child2Exists := erl.WhereIs(supChild2)
	assert.Assert(t, !child2Exists)
	assert.Assert(t, !erl.IsAlive(child2PID))
}

func TestStartLink_ReturnsExceptionIfAnyChildErrors(t *testing.T) {
	sup := TestSup{
		supFlags: NewSupFlags(),
		childSpecs: []ChildSpec{
			NewChildSpec("child1",
				testSrvStartFun[TestSrvState](supChild1, nil)),
			NewChildSpec("child2",
				testSrvStartFun[TestSrvState](supChild2, nil,
					genserver.SetInitProbe(
						func(self erl.PID, args any) (TestSrvState, any, error) {
							return TestSrvState{}, nil, errors.New("not today")
						},
					))),
		},
	}
	pid, err := testStartSupervisor(t, sup, nil)
	erl.Logger.Printf("Supervisor PID: %v", pid)
	erl.Logger.Printf("Supervisor Error: %v", err)

	assert.Assert(t, exitreason.IsShutdown(err))
	assert.ErrorContains(t, err, "not today")
	assert.Assert(t, !erl.IsAlive(pid))

	child1PID, child1Exists := erl.WhereIs(supChild1)
	assert.Assert(t, !child1Exists)
	assert.Assert(t, !erl.IsAlive(child1PID))

	child2PID, child2Exists := erl.WhereIs(supChild2)
	assert.Assert(t, !child2Exists)
	assert.Assert(t, !erl.IsAlive(child2PID))
}

func TestStartLink_ReturnsExceptionIfStartFunPanics(t *testing.T) {
	sup := TestSup{
		supFlags: NewSupFlags(),
		childSpecs: []ChildSpec{
			NewChildSpec("child1",
				testSrvStartFun[TestSrvState](supChild1, nil)),
			NewChildSpec("child2", func(sup erl.PID) (erl.PID, error) {
				panic("uh-oh")
			}),
		},
	}
	pid, err := testStartSupervisor(t, sup, nil)
	erl.Logger.Printf("Supervisor PID: %v", pid)
	erl.Logger.Printf("Supervisor Error: %v", err)

	assert.Assert(t, exitreason.IsException(err))
	assert.ErrorContains(t, err, "uh-oh")
	assert.Assert(t, !erl.IsAlive(pid))

	child1PID, child1Exists := erl.WhereIs(supChild1)
	assert.Assert(t, !child1Exists)
	assert.Assert(t, !erl.IsAlive(child1PID))

	child2PID, child2Exists := erl.WhereIs(supChild2)
	assert.Assert(t, !child2Exists)
	assert.Assert(t, !erl.IsAlive(child2PID))
}

func TestStartLink_ReturnsExceptionIfDuplicateChildIDs(t *testing.T) {
	sup := TestSup{
		supFlags: NewSupFlags(),
		childSpecs: []ChildSpec{
			NewChildSpec("child1",
				testSrvStartFun[TestSrvState](supChild1, nil)),
			NewChildSpec("child1",
				testSrvStartFun[TestSrvState](supChild2, nil)),
		},
	}

	pid, err := testStartSupervisor(t, sup, nil)

	assert.Error(t, err, "EXIT{shutdown: duplicate childspec id found: child1}")
	assert.Assert(t, !erl.IsAlive(pid))

	child1PID, child1Exists := erl.WhereIs(supChild1)
	assert.Assert(t, !child1Exists)
	assert.Assert(t, !erl.IsAlive(child1PID))

	child2PID, child2Exists := erl.WhereIs(supChild2)
	assert.Assert(t, !child2Exists)
	assert.Assert(t, !erl.IsAlive(child2PID))
}

func TestSupervisorS_RestartsChildrenWithPermanentStrategy(t *testing.T) {
	trPID, tr := erl.NewTestReceiver(t)
	sup := TestSup{
		supFlags: NewSupFlags(),
		childSpecs: []ChildSpec{
			NewChildSpec("child1",
				testSrvStartFun[TestSrvState](
					supChild1,
					nil,
					genserver.SetTestReceiver[TestSrvState](trPID),
					genserver.SetInitialState[TestSrvState](TestSrvState{id: "child1"}),
				)),
			NewChildSpec("child2",
				testSrvStartFun[TestSrvState](
					supChild2,
					nil,
					genserver.SetTestReceiver[TestSrvState](trPID),
					genserver.SetInitialState[TestSrvState](TestSrvState{id: "child2"}),
				)),
		},
	}
	_, err := testStartSupervisor(t, sup, nil)

	assert.NilError(t, err)

	child1PID, _ := erl.WhereIs(supChild1)
	// erl.Monitor(tr.Self(), child1PID)
	child2PID, _ := erl.WhereIs(supChild2)

	var gotNotif1 bool
	tr.Loop(func(anyMsg any) bool {
		switch msg := anyMsg.(type) {
		case genserver.TestNotifInit[TestSrvState]:
			if msg.State.id == "child1" {
				gotNotif1 = true
			}
		default:
			// ignore
		}

		return gotNotif1
	})

	t.Log("got init message from child1, now sending exit")
	erl.Exit(erl.RootPID(), child1PID, exitreason.Kill)

	var gotExit bool
	var gotRestartInit bool
	tr.Loop(func(anyMsg any) bool {
		switch msg := anyMsg.(type) {
		case erl.ExitMsg:
			if msg.Proc.Equals(child1PID) {
				gotExit = true
			}
		case genserver.TestNotifInit[TestSrvState]:
			if msg.State.id == "child1" {
				gotRestartInit = true
			}
		default:
			// ignore
		}

		return gotExit && gotRestartInit
	})

	child1PIDAgain, ok := erl.WhereIs(supChild1)
	assert.Assert(t, ok)
	child2PIDAgain, _ := erl.WhereIs(supChild2)

	assert.Assert(t, child2PID.Equals(child2PIDAgain))
	assert.Assert(t, !child1PID.Equals(child1PIDAgain))
}

func TestSupervisorS_RestartsChildrenWithTransientStrategyIfExceptionExit(t *testing.T) {
	trPID, tr := erl.NewTestReceiver(t)
	sup := TestSup{
		supFlags: NewSupFlags(),
		childSpecs: []ChildSpec{
			NewChildSpec("child1",
				testSrvStartFun[TestSrvState](
					supChild1,
					nil,
					genserver.SetTestReceiver[TestSrvState](trPID),
					genserver.SetInitialState[TestSrvState](TestSrvState{id: "child1"}),
				), SetRestart(Transient)),
			NewChildSpec("child2",
				testSrvStartFun[TestSrvState](
					supChild2,
					nil,
					genserver.SetTestReceiver[TestSrvState](trPID),
					genserver.SetInitialState[TestSrvState](TestSrvState{id: "child2"}),
				), SetRestart(Transient)),
		},
	}

	_, err := testStartSupervisor(t, sup, nil)

	assert.NilError(t, err)

	child1PID, _ := erl.WhereIs(supChild1)
	child2PID, _ := erl.WhereIs(supChild2)
	var gotInit1 bool
	tr.Loop(func(anyMsg any) bool {
		switch msg := anyMsg.(type) {
		case genserver.TestNotifInit[TestSrvState]:
			if msg.State.id == "child1" {
				gotInit1 = true
			}
		default:
			// ignore
		}

		return gotInit1
	})

	t.Log("got init message from child1, now sending exit")

	erl.Exit(erl.RootPID(), child1PID, exitreason.Kill)

	var gotExit bool
	var gotRestartInit bool
	tr.Loop(func(anyMsg any) bool {
		switch msg := anyMsg.(type) {
		case erl.ExitMsg:
			if msg.Proc.Equals(child1PID) {
				gotExit = true
			}
		case genserver.TestNotifInit[TestSrvState]:
			if msg.State.id == "child1" {
				gotRestartInit = true
			}
		default:
			// ignore
		}

		return gotExit && gotRestartInit
	})

	child1PIDAgain, ok := erl.WhereIs(supChild1)
	assert.Assert(t, ok)
	child2PIDAgain, ok := erl.WhereIs(supChild2)
	assert.Assert(t, ok)

	assert.Assert(t, child2PID.Equals(child2PIDAgain))
	assert.Assert(t, !child1PID.Equals(child1PIDAgain))
}

func TestSupervisorS_DoNotRestartTransientChildWhenExitNormal(t *testing.T) {
	trPID, tr := erl.NewTestReceiver(t)
	sup := TestSup{
		supFlags: NewSupFlags(),
		childSpecs: []ChildSpec{
			NewChildSpec("child1",
				testSrvStartFun[TestSrvState](
					supChild1,
					nil,
					genserver.SetTestReceiver[TestSrvState](trPID),
					genserver.SetInitialState[TestSrvState](TestSrvState{id: "child1"}),
				), SetRestart(Transient)),
			NewChildSpec("child2",
				testSrvStartFun[TestSrvState](
					supChild2,
					nil,
					genserver.SetTestReceiver[TestSrvState](trPID),
					genserver.SetInitialState[TestSrvState](TestSrvState{id: "child2"}),
				), SetRestart(Transient)),
		},
	}

	supPID, err := testStartSupervisor(t, sup, nil)
	assert.NilError(t, err)

	child1PID, _ := erl.WhereIs(supChild1)
	child2PID, _ := erl.WhereIs(supChild2)

	msg := genserver.NewTestMsg[TestSrvState](genserver.SetCallProbe[TestSrvState](
		func(self erl.PID, arg any, from genserver.From, state TestSrvState) (genserver.CallResult[TestSrvState], error) {
			return genserver.CallResult[TestSrvState]{Msg: "signing off", State: state}, exitreason.Normal
		},
	))
	result, err := genserver.Call(trPID, child1PID, msg, chronos.Dur("5s"))
	assert.NilError(t, err)
	assert.DeepEqual(t, result, "signing off")

	var childDead bool
	tr.Loop(func(anymsg any) bool {
		switch msg := anymsg.(type) {
		case erl.ExitMsg:
			if msg.Proc.Equals(child1PID) {
				childDead = true
			}
		}

		return childDead
	})

	assert.Assert(t, erl.IsAlive(supPID))
	_, ok := erl.WhereIs(supChild1)
	assert.Assert(t, !ok)
	child2PIDAgain, _ := erl.WhereIs(supChild2)
	assert.Assert(t, child2PID.Equals(child2PIDAgain))
}

func TestSupervisorS_DoNotRestartTransientChildWhenExitShutdown(t *testing.T) {
	trPID, tr := erl.NewTestReceiver(t)
	sup := TestSup{
		supFlags: NewSupFlags(),
		childSpecs: []ChildSpec{
			NewChildSpec("child1",
				testSrvStartFun[TestSrvState](
					supChild1,
					nil,
					genserver.SetTestReceiver[TestSrvState](trPID),
					genserver.SetInitialState[TestSrvState](TestSrvState{id: "child1"}),
				), SetRestart(Transient)),
			NewChildSpec("child2",
				testSrvStartFun[TestSrvState](
					supChild2,
					nil,
					genserver.SetTestReceiver[TestSrvState](trPID),
					genserver.SetInitialState[TestSrvState](TestSrvState{id: "child2"}),
				), SetRestart(Transient)),
		},
	}

	supPID, err := testStartSupervisor(t, sup, nil)
	assert.NilError(t, err)

	child1PID, _ := erl.WhereIs(supChild1)
	child2PID, _ := erl.WhereIs(supChild2)

	msg := genserver.NewTestMsg[TestSrvState](genserver.SetCallProbe[TestSrvState](
		func(self erl.PID, arg any, from genserver.From, state TestSrvState) (genserver.CallResult[TestSrvState], error) {
			return genserver.CallResult[TestSrvState]{Msg: "signing off", State: state}, exitreason.Shutdown("shut it down!")
		},
	))
	result, err := genserver.Call(trPID, child1PID, msg, chronos.Dur("5s"))
	assert.NilError(t, err)
	assert.DeepEqual(t, result, "signing off")

	var childDead bool
	tr.Loop(func(anymsg any) bool {
		switch msg := anymsg.(type) {
		case erl.ExitMsg:
			if msg.Proc.Equals(child1PID) {
				childDead = true
			}
		}

		return childDead
	})

	assert.Assert(t, erl.IsAlive(supPID))
	_, ok := erl.WhereIs(supChild1)
	assert.Assert(t, !ok)
	child2PIDAgain, _ := erl.WhereIs(supChild2)
	assert.Assert(t, child2PID.Equals(child2PIDAgain))
}

func TestSupervisorS_DoNotRestartTransientChildWhenExitSupervisorShutdown(t *testing.T) {
	trPID, tr := erl.NewTestReceiver(t)
	sup := TestSup{
		supFlags: NewSupFlags(),
		childSpecs: []ChildSpec{
			NewChildSpec("child1",
				testSrvStartFun[TestSrvState](
					supChild1,
					nil,
					genserver.SetTestReceiver[TestSrvState](trPID),
					genserver.SetInitialState[TestSrvState](TestSrvState{id: "child1"}),
				), SetRestart(Transient)),
			NewChildSpec("child2",
				testSrvStartFun[TestSrvState](
					supChild2,
					nil,
					genserver.SetTestReceiver[TestSrvState](trPID),
					genserver.SetInitialState[TestSrvState](TestSrvState{id: "child2"}),
				), SetRestart(Transient)),
		},
	}

	supPID, err := testStartSupervisor(t, sup, nil)
	assert.NilError(t, err)

	child1PID, _ := erl.WhereIs(supChild1)
	child2PID, _ := erl.WhereIs(supChild2)

	msg := genserver.NewTestMsg[TestSrvState](genserver.SetCallProbe[TestSrvState](
		func(self erl.PID, arg any, from genserver.From, state TestSrvState) (genserver.CallResult[TestSrvState], error) {
			return genserver.CallResult[TestSrvState]{Msg: "signing off", State: state}, exitreason.SupervisorShutdown
		},
	))
	result, err := genserver.Call(trPID, child1PID, msg, chronos.Dur("5s"))
	assert.NilError(t, err)
	assert.DeepEqual(t, result, "signing off")

	var childDead bool
	tr.Loop(func(anymsg any) bool {
		switch msg := anymsg.(type) {
		case erl.ExitMsg:
			if msg.Proc.Equals(child1PID) {
				childDead = true
			}
		}

		return childDead
	})

	assert.Assert(t, erl.IsAlive(supPID))
	_, ok := erl.WhereIs(supChild1)
	assert.Assert(t, !ok)
	child2PIDAgain, _ := erl.WhereIs(supChild2)
	assert.Assert(t, child2PID.Equals(child2PIDAgain))
}

func TestSupervisorS_RestartIntensityReachedCrashesSupervisor(t *testing.T) {
	trPID, tr := erl.NewTestReceiver(t)
	sup := TestSup{
		supFlags: NewSupFlags(),
		childSpecs: []ChildSpec{
			NewChildSpec("child1",
				testSrvStartFun[TestSrvState](
					supChild1,
					nil,
					genserver.SetTestReceiver[TestSrvState](trPID),
					genserver.SetInitialState[TestSrvState](TestSrvState{id: "child1"}),
				)),
			NewChildSpec("child2",
				testSrvStartFun[TestSrvState](
					supChild2,
					nil,
					genserver.SetTestReceiver[TestSrvState](trPID),
					genserver.SetInitialState[TestSrvState](TestSrvState{id: "child2"}),
				)),
		},
	}
	supPID, err := testStartSupervisor(t, sup, nil)
	erl.Link(trPID, supPID)

	assert.NilError(t, err)
	child1PID, _ := erl.WhereIs(supChild1)
	// erl.Monitor(tr.Self(), child1PID)
	child2PID, _ := erl.WhereIs(supChild2)

	erl.Exit(trPID, child1PID, exitreason.Kill)
	erl.Exit(trPID, child2PID, exitreason.Kill)

	var supDead bool
	tr.Loop(func(anymsg any) bool {
		switch msg := anymsg.(type) {
		case erl.ExitMsg:
			if msg.Proc.Equals(supPID) {
				supDead = true
			}
		default:
			// ignore
		}
		return supDead
	})
	assert.Assert(t, !erl.IsAlive(supPID))
	_, ok := erl.WhereIs(supName)
	assert.Assert(t, !ok)
}

func TestSupervisorS_DoNotRestartTemporaryChildren(t *testing.T) {
	trPID, tr := erl.NewTestReceiver(t)
	sup := TestSup{
		supFlags: NewSupFlags(),
		childSpecs: []ChildSpec{
			NewChildSpec("child1",
				testSrvStartFun[TestSrvState](
					supChild1,
					nil,
					genserver.SetTestReceiver[TestSrvState](trPID),
					genserver.SetInitialState[TestSrvState](TestSrvState{id: "child1"}),
				), SetRestart(Temporary)),
			NewChildSpec("child2",
				testSrvStartFun[TestSrvState](
					supChild2,
					nil,
					genserver.SetTestReceiver[TestSrvState](trPID),
					genserver.SetInitialState[TestSrvState](TestSrvState{id: "child2"}),
				), SetRestart(Temporary)),
		},
	}

	supPID, err := testStartSupervisor(t, sup, nil)
	assert.NilError(t, err)

	child1PID, _ := erl.WhereIs(supChild1)
	child2PID, _ := erl.WhereIs(supChild2)

	msg := genserver.NewTestMsg[TestSrvState](genserver.SetCallProbe[TestSrvState](
		func(self erl.PID, arg any, from genserver.From, state TestSrvState) (genserver.CallResult[TestSrvState], error) {
			return genserver.CallResult[TestSrvState]{Msg: "signing off", State: state}, exitreason.Normal
		},
	))
	result, err := genserver.Call(trPID, child1PID, msg, chronos.Dur("5s"))
	assert.NilError(t, err)
	assert.DeepEqual(t, result, "signing off")

	var childDead bool
	tr.Loop(func(anymsg any) bool {
		switch msg := anymsg.(type) {
		case erl.ExitMsg:
			if msg.Proc.Equals(child1PID) {
				childDead = true
			}
		}

		return childDead
	})

	assert.Assert(t, erl.IsAlive(supPID))
	_, ok := erl.WhereIs(supChild1)
	assert.Assert(t, !ok)
	child2PIDAgain, _ := erl.WhereIs(supChild2)
	assert.Assert(t, child2PID.Equals(child2PIDAgain))
}

func TestSupervisorS_StrategyOneForAll_CrashAllProcesses(t *testing.T) {
	trPID, tr := erl.NewTestReceiver(t)
	sup := TestSup{
		supFlags: NewSupFlags(SetStrategy(OneForAll)),
		childSpecs: []ChildSpec{
			NewChildSpec("child1",
				testSrvStartFun[TestSrvState](
					supChild1,
					nil,
					genserver.SetTestReceiver[TestSrvState](trPID),
					genserver.SetInitialState[TestSrvState](TestSrvState{id: "child1"}),
				)),
			NewChildSpec("child2",
				testSrvStartFun[TestSrvState](
					supChild2,
					nil,
					genserver.SetTestReceiver[TestSrvState](trPID),
					genserver.SetInitialState[TestSrvState](TestSrvState{id: "child2"}),
				)),
		},
	}

	supPID, err := testStartSupervisor(t, sup, nil)
	assert.NilError(t, err)

	child1PID, _ := erl.WhereIs(supChild1)
	child2PID, _ := erl.WhereIs(supChild2)

	msg := genserver.NewTestMsg[TestSrvState](genserver.SetCallProbe[TestSrvState](
		func(self erl.PID, arg any, from genserver.From, state TestSrvState) (genserver.CallResult[TestSrvState], error) {
			return genserver.CallResult[TestSrvState]{Msg: "signing off", State: state}, exitreason.Normal
		},
	))
	result, err := genserver.Call(trPID, child1PID, msg, chronos.Dur("5s"))
	assert.NilError(t, err)
	assert.DeepEqual(t, result, "signing off")

	var child1Dead bool
	var child2Dead bool
	tr.Loop(func(anymsg any) bool {
		switch msg := anymsg.(type) {
		case erl.ExitMsg:
			if msg.Proc.Equals(child1PID) {
				child1Dead = true
			}
			if msg.Proc.Equals(child2PID) {
				child2Dead = true
			}
		default:
			// ignore
		}

		return child1Dead && child2Dead
	})

	var child1Restarted bool
	var child2Restarted bool
	tr.Loop(func(anymsg any) bool {
		switch msg := anymsg.(type) {
		case genserver.TestNotifInit[TestSrvState]:
			if !child2Restarted {
				child2PID, ok := erl.WhereIs(supChild2)
				if ok {
					t.Logf("test if child2PID %v matches %v", child2PID, msg.Self)
					child2Restarted = child2PID.Equals(msg.Self)
					t.Logf("Child2Restarted %t", child2Restarted)
				}
			}
			if !child1Restarted {
				child1PID, ok := erl.WhereIs(supChild1)
				if ok {
					t.Logf("test if child1PID %v matches %v", child1PID, msg.Self)
					child1Restarted = child1PID.Equals(msg.Self)
					t.Logf("Child1Restarted %t", child1Restarted)
				}
			}

		default:
			// ignore
		}
		return child2Restarted && child1Restarted
	})

	// restarting all the supervisors should not crash supervisors from restart intensity
	assert.Assert(t, erl.IsAlive(supPID))
	child1Again, ok := erl.WhereIs(supChild1)
	assert.Assert(t, ok)
	assert.Assert(t, !child1PID.Equals(child1Again))
	child2PIDAgain, _ := erl.WhereIs(supChild2)
	assert.Assert(t, !child2PID.Equals(child2PIDAgain))
}

func TestSupervisorS_StrategyRestForOne_CrashAllProcessesAfterCrashedProcChildSpec(t *testing.T) {
	trPID, tr := erl.NewTestReceiver(t)
	sup := TestSup{
		supFlags: NewSupFlags(SetStrategy(RestForOne)),
		childSpecs: []ChildSpec{
			NewChildSpec("child1",
				testSrvStartFun[TestSrvState](
					supChild1,
					nil,
					genserver.SetTestReceiver[TestSrvState](trPID),
					genserver.SetInitialState[TestSrvState](TestSrvState{id: "child1"}),
				)),
			NewChildSpec("child2",
				testSrvStartFun[TestSrvState](
					supChild2,
					nil,
					genserver.SetTestReceiver[TestSrvState](trPID),
					genserver.SetInitialState[TestSrvState](TestSrvState{id: "child2"}),
				)),
			NewChildSpec("child3",
				testSrvStartFun[TestSrvState](
					supChild3,
					nil,
					genserver.SetTestReceiver[TestSrvState](trPID),
					genserver.SetInitialState[TestSrvState](TestSrvState{id: "child3"}),
				)),
			NewChildSpec("child4",
				testSrvStartFun[TestSrvState](
					supChild4,
					nil,
					genserver.SetTestReceiver[TestSrvState](trPID),
					genserver.SetInitialState[TestSrvState](TestSrvState{id: "child4"}),
				)),
		},
	}

	supPID, err := testStartSupervisor(t, sup, nil)
	assert.NilError(t, err)

	child1PID, _ := erl.WhereIs(supChild1)
	child2PID, _ := erl.WhereIs(supChild2)
	child3PID, _ := erl.WhereIs(supChild3)
	child4PID, _ := erl.WhereIs(supChild4)

	msg := genserver.NewTestMsg[TestSrvState](genserver.SetCallProbe[TestSrvState](
		func(self erl.PID, arg any, from genserver.From, state TestSrvState) (genserver.CallResult[TestSrvState], error) {
			return genserver.CallResult[TestSrvState]{Msg: "signing off", State: state}, exitreason.Normal
		},
	))
	result, err := genserver.Call(trPID, child2PID, msg, chronos.Dur("5s"))
	assert.NilError(t, err)
	assert.DeepEqual(t, result, "signing off")

	var child2Dead bool
	var child3Dead bool
	var child4Dead bool
	tr.Loop(func(anymsg any) bool {
		switch msg := anymsg.(type) {
		case erl.ExitMsg:
			if msg.Proc.Equals(child1PID) {
				t.Fatal("child1 should not have restarted")
			}
			if msg.Proc.Equals(child2PID) {
				child2Dead = true
			}
			if msg.Proc.Equals(child3PID) {
				child3Dead = true
			}
			if msg.Proc.Equals(child4PID) {
				child4Dead = true
			}
		default:
			// ignore
		}

		return child2Dead && child3Dead && child4Dead
	})

	var child2Restarted bool
	var child3Restarted bool
	var child4Restarted bool
	tr.Loop(func(anymsg any) bool {
		switch msg := anymsg.(type) {
		case genserver.TestNotifInit[TestSrvState]:
			if !child2Restarted {
				child2PID, ok := erl.WhereIs(supChild2)
				if ok {
					t.Logf("test if child2PID %v matches %v", child2PID, msg.Self)
					child2Restarted = child2PID.Equals(msg.Self)
					t.Logf("Child2Restarted %t", child2Restarted)
				}
			}
			if !child3Restarted {
				child3PID, ok := erl.WhereIs(supChild3)
				if ok {
					t.Logf("test if child3PID %v matches %v", child3PID, msg.Self)
					child3Restarted = child3PID.Equals(msg.Self)
					t.Logf("Child3Restarted %t", child3Restarted)
				}
			}
			if !child4Restarted {
				child4PID, ok := erl.WhereIs(supChild4)
				if ok {
					t.Logf("test if child4PID %v matches %v", child4PID, msg.Self)
					child4Restarted = child4PID.Equals(msg.Self)
					t.Logf("Child4Restarted %t", child4Restarted)
				}
			}

		default:
			// ignore
		}
		return child2Restarted && child3Restarted && child4Restarted
	})

	assert.Assert(t, erl.IsAlive(supPID))
	child1Again, ok := erl.WhereIs(supChild1)
	assert.Assert(t, ok)
	assert.Assert(t, child1PID.Equals(child1Again))
}
