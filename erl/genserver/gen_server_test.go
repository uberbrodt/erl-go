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

func TestGenServer_Call_Success(t *testing.T) {
	args := TestGSArgs{count: 2}

	tr, _ := NewTestReceiver(t)
	pid, err := startTestGS(tr, t, TestGS{}, args)
	assert.NilError(t, err)
	erl.Link(tr, pid)

	gensrvPID, _ := StartLink[TestGS](tr, TestGS{}, args)

	reply, err := Call(tr, gensrvPID,
		taggedRequest{
			tag:   "anything",
			value: 12,
			callProbe: func(self erl.PID, arg any, from From, state TestGS) (any, TestGS) {
				val := arg.(int)
				state.Count += val
				return state.Count, state
			},
		}, chronos.Dur("5s"))

	t.Logf("reply: %+v", reply)
	assert.NilError(t, err)
	intReply := reply.(int)
	assert.Equal(t, intReply, 14)
}

func TestGenServer_Call_ErrorNoProc(t *testing.T) {
	args := TestGSArgs{count: 2}

	name := erl.Name("myPid3")

	Start[TestGS](erl.RootPID(), TestGS{}, args, SetName(name))

	reply, err := Call(erl.RootPID(), erl.Name("myPid4"), taggedRequest{tag: "anything", value: "my call arg"}, chronos.Dur("5s"))

	assert.ErrorIs(t, err, exitreason.NoProc)
	assert.Assert(t, reply == nil)
}

func TestGenServer_Call_StopAndReply(t *testing.T) {
	args := TestGSArgs{count: 2}

	tr, receive := NewTestReceiver(t)
	gensrvPID, err := startTestGS(tr, t, TestGS{}, args)
	assert.NilError(t, err)

	reply, err := Call(tr, gensrvPID,
		taggedRequest{
			tag:   "anything",
			value: 12,
			err:   exitreason.Exception(errors.New("some error")),
			callProbe: func(self erl.PID, arg any, from From, state TestGS) (any, TestGS) {
				val := arg.(int)
				state.Count += val
				return state.Count, state
			},
		}, chronos.Dur("5s"))

	assert.NilError(t, err)
	intReply := reply.(int)
	assert.Equal(t, intReply, 14)

	anymsg := <-receive
	switch msg := anymsg.(type) {
	case erl.ExitMsg:
		assert.Assert(t, msg.Proc.Equals(gensrvPID))
	}
	assert.Assert(t, erl.IsAlive(gensrvPID) == false)
}

func TestGenServer_Call_NoReplyStop(t *testing.T) {
	args := TestGSArgs{count: 2}
	tr, _ := NewTestReceiver(t)
	gensrvPID, err := startTestGS(tr, t, TestGS{}, args)
	assert.NilError(t, err)

	reply, err := Call(tr, gensrvPID,
		taggedRequest{
			tag:   "anything",
			value: 12,
			err:   exitreason.Shutdown(errors.New("some error")),
			callProbe: func(self erl.PID, arg any, from From, state TestGS) (any, TestGS) {
				val := arg.(int)
				state.Count += val
				return nil, state
			},
		}, chronos.Dur("5s"))

	assert.ErrorIs(t, err, exitreason.Stopped)
	assert.Assert(t, reply == nil)
	assert.Assert(t, erl.IsAlive(gensrvPID) == false)
}

func TestGenServer_Call_NoReplyContinue_Reply(t *testing.T) {
	args := TestGSArgs{count: 2}

	tr, _ := NewTestReceiver(t)
	gensrvPID, err := startTestGS(tr, t, TestGS{}, args)
	assert.NilError(t, err)

	reply, err := Call(erl.RootPID(), gensrvPID,
		taggedRequest{
			cont:  true,
			value: 12,
			callProbe: func(self erl.PID, arg any, from From, state TestGS) (any, TestGS) {
				val := arg.(int)
				state.Count += val
				state.from = from
				return nil, state
			},
			continueProbe: func(self erl.PID, state TestGS) (TestGS, error) {
				Reply(state.from, state.Count+1)
				return state, nil
			},
		}, chronos.Dur("5s"))

	assert.NilError(t, err)
	intReply := reply.(int)
	assert.Equal(t, intReply, 15)
}

func TestGenServer_Call_NoReplyContinue_ExitWithTimeout(t *testing.T) {
	args := TestGSArgs{count: 2}

	tr, _ := NewTestReceiver(t)
	gensrvPID, err := startTestGS(tr, t, TestGS{}, args)
	assert.NilError(t, err)

	reply, err := Call(erl.RootPID(), gensrvPID,
		taggedRequest{
			cont:  true,
			value: 12,
			callProbe: func(self erl.PID, arg any, from From, state TestGS) (any, TestGS) {
				val := arg.(int)
				state.Count += val
				state.from = from
				return nil, state
			},
			continueProbe: func(self erl.PID, state TestGS) (TestGS, error) {
				time.After(chronos.Dur("10s"))
				return state, nil
			},
		}, chronos.Dur("200ms"))

	assert.ErrorIs(t, err, exitreason.Timeout)
	assert.Assert(t, reply == nil)
}

func TestGenServer_Init_RegisterNameSuccess(t *testing.T) {
	args := TestGSArgs{count: 2}
	name := erl.Name("myPid")

	gensrvPID, err := Start[TestGS](erl.RootPID(), TestGS{}, args, SetName(name))
	assert.NilError(t, err)

	regPID, regErr := erl.WhereIs(name)
	assert.Assert(t, regErr == true)
	assert.Assert(t, gensrvPID == regPID)
}

func TestGenServer_Init_RegisterNameAlreadyRegistered(t *testing.T) {
	args := TestGSArgs{count: 2}
	name := erl.Name("myPid2")

	gensrvPID, err := Start[TestGS](erl.RootPID(), TestGS{}, args, SetName(name))
	t.Logf("error %+v", err)
	assert.NilError(t, err)

	regPID, regErr := erl.WhereIs(name)
	assert.Assert(t, regErr == true)
	assert.Assert(t, gensrvPID == regPID)

	_, err = Start[TestGS](erl.RootPID(), TestGS{}, args, SetName(name))
	assert.ErrorContains(t, err, "name_used")
}

func TestGenServer_Init_ReturnsStop(t *testing.T) {
	trPID, tr := erl.NewTestReceiver(t)
	args := TestGSArgs{
		count: 2,
		initProbe: func(self erl.PID, args any) (state TestGS, cont any, err error) {
			return TestGS{Count: 2}, nil, errors.New("random init error")
		},
	}
	name := erl.Name("myPid4")

	genSrvPID, err := StartLink[TestGS](trPID, TestGS{}, args, SetName(name))
	assert.Assert(t, exitreason.IsException(err))
	assert.ErrorContains(t, err, "random init error")

	success := tr.Loop(func(msg any) bool {
		xit, ok := msg.(erl.ExitMsg)

		if !ok {
			return false
		}

		if xit.Proc.Equals(genSrvPID) {
			return true
		}
		return false
	})

	assert.Assert(t, success)

	assert.Assert(t, !erl.IsAlive(genSrvPID))
}

func TestGenServer_Init_ReturnsIgnore(t *testing.T) {
	trPID, tr := erl.NewTestReceiver(t)
	args := TestGSArgs{
		count: 2,
		initProbe: func(self erl.PID, args any) (state TestGS, cont any, err error) {
			return TestGS{Count: 2}, nil, exitreason.Ignore
		},
	}
	name := erl.Name("myPid8")

	genSrvPID, err := StartLink[TestGS](trPID, TestGS{}, args, SetName(name))
	assert.ErrorIs(t, err, exitreason.Ignore)

	success := tr.Loop(func(msg any) bool {
		xit, ok := msg.(erl.ExitMsg)

		if !ok {
			return false
		}

		if xit.Proc.Equals(genSrvPID) {
			return true
		}
		return false
	})

	assert.Assert(t, success)
	assert.Assert(t, !erl.IsAlive(genSrvPID))
}

func TestGenServer_Init_Continues(t *testing.T) {
	tr, receive := NewTestReceiver(t)
	args := TestGSArgs{
		count: 2,
		initProbe: func(self erl.PID, args any) (state TestGS, cont any, err error) {
			return TestGS{Count: 2}, taggedRequest{
				continueProbe: func(self erl.PID, state TestGS) (TestGS, error) {
					receive <- "continue"
					return state, nil
				},
			}, nil
		},
	}
	gensrvPID, err := startTestGS(tr, t, TestGS{}, args)
	assert.NilError(t, err)
	assert.Assert(t, erl.IsAlive(gensrvPID))
	anyMsg := <-receive

	msg := anyMsg.(string)

	assert.Equal(t, msg, "continue")
}

func TestGenServer_Cast_DoesContinue(t *testing.T) {
	pid, err := startTestGS(erl.RootPID(), t, TestGS{}, TestGSArgs{})
	receive := make(chan int, 1)
	assert.NilError(t, err)

	Cast(pid, taggedRequest{
		cont: true,
		continueProbe: func(self erl.PID, state TestGS) (TestGS, error) {
			receive <- 2
			return state, nil
		},
	})
	select {
	case intMsg := <-receive:
		assert.Equal(t, intMsg, 2)

	case <-time.After(chronos.Dur("5s")):
		t.Fatalf("timed out waiting for probe response")
	}
}

func TestGenServer_Cast_DoesStopOnError(t *testing.T) {
	tr, receive := NewTestReceiver(t)
	pid, err := startTestGS(tr, t, TestGS{terminateProbe: func(self erl.PID, exit error, state TestGS) {
		assert.ErrorIs(t, exit, exitreason.Normal)
		receive <- 1
	}}, TestGSArgs{})
	assert.NilError(t, err)

	Cast(pid, taggedRequest{
		err: exitreason.Normal,
	})
	terminateCalled := false
	done := false
	for !done {
		select {
		case anyMsg := <-receive:
			switch msg := anyMsg.(type) {
			case int:
				assert.Equal(t, msg, 1)
				terminateCalled = true
			case erl.ExitMsg:
				assert.Assert(t, msg.Proc.Equals(pid))
				assert.Assert(t, !erl.IsAlive(pid))
				done = true
			}

		case <-time.After(chronos.Dur("5s")):
			t.Fatalf("timed out waiting for probe response")
		}
	}
	assert.Assert(t, terminateCalled)
}

func TestGenServer_Info_Success(t *testing.T) {
	pid, err := startTestGS(erl.RootPID(), t, TestGS{}, TestGSArgs{})
	receive := make(chan int, 1)
	assert.NilError(t, err)

	erl.Send(pid, taggedRequest{
		probe: func(self erl.PID, state TestGS) TestGS {
			receive <- 2
			return state
		},
	})
	select {
	case intMsg := <-receive:
		assert.Equal(t, intMsg, 2)

	case <-time.After(chronos.Dur("5s")):
		t.Fatalf("timed out waiting for probe response")
	}
}

func TestGenServer_Info_DoesContinue(t *testing.T) {
	pid, err := startTestGS(erl.RootPID(), t, TestGS{}, TestGSArgs{})
	receive := make(chan int, 1)
	assert.NilError(t, err)

	erl.Send(pid, taggedRequest{
		cont: true,
		continueProbe: func(self erl.PID, state TestGS) (TestGS, error) {
			receive <- 2
			return state, nil
		},
	})
	select {
	case intMsg := <-receive:
		assert.Equal(t, intMsg, 2)

	case <-time.After(chronos.Dur("5s")):
		t.Fatalf("timed out waiting for probe response")
	}
}

func TestGenServer_Info_DoesStopOnError(t *testing.T) {
	tr, receive := NewTestReceiver(t)
	pid, err := startTestGS(tr, t, TestGS{terminateProbe: func(self erl.PID, exit error, state TestGS) {
		assert.ErrorIs(t, exit, exitreason.Normal)
		receive <- 1
	}}, TestGSArgs{})
	assert.NilError(t, err)

	erl.Send(pid, taggedRequest{
		err: exitreason.Normal,
	})
	terminateCalled := false
	done := false
	for !done {
		select {
		case anyMsg := <-receive:
			switch msg := anyMsg.(type) {
			case int:
				assert.Equal(t, msg, 1)
				terminateCalled = true
			case erl.ExitMsg:
				assert.Assert(t, msg.Proc.Equals(pid))
				assert.Assert(t, !erl.IsAlive(pid))
				done = true
			}

		case <-time.After(chronos.Dur("5s")):
			t.Fatalf("timed out waiting for probe response")
		}
	}
	assert.Assert(t, terminateCalled)
}