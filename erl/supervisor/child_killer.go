package supervisor

import (
	"errors"
	"fmt"
	"time"

	"github.com/uberbrodt/erl-go/chronos"
	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
)

type childKillerDoneMsg struct {
	err error
}

type childKiller struct {
	parent     chan<- childKillerDoneMsg
	parentPID  erl.PID
	child      ChildSpec
	monitorRef erl.Ref
}

func (ck *childKiller) Receive(self erl.PID, inbox <-chan any) error {
	erl.DebugPrintf("Supervisor %v is terminating %+v", ck.parentPID, ck.child)
	ck.monitorRef = erl.Monitor(self, ck.child.pid)
	// unlink the supervisor so it doesn't get an ExitMsg
	erl.Unlink(ck.parentPID, ck.child.pid)

	switch shutdown := ck.child.Shutdown; {
	case shutdown.BrutalKill:
		ck.handleBrutalKill(self, inbox)
		return exitreason.Normal
	case shutdown.Infinity:
		erl.Exit(self, ck.child.pid, exitreason.SupervisorShutdown)
		for anyMsg := range inbox {
			switch msg := anyMsg.(type) {
			case erl.DownMsg:
				ck.handleDown(self, msg)
				return exitreason.Normal
			default:
				erl.DebugPrintf("childkiller[%s]: got a messsage that wasn't erl.DownMsg: %+v", ck.child.ID, msg)
			}
		}
	default:
		ck.handleTimeout(self, inbox)
		return exitreason.Normal
	}
	return exitreason.Normal
}

func (ck *childKiller) handleBrutalKill(self erl.PID, inbox <-chan any) {
	erl.Exit(ck.parentPID, ck.child.pid, exitreason.Kill)
	for anyMsg := range inbox {
		switch msg := anyMsg.(type) {
		case erl.DownMsg:
			if msg.Ref != ck.monitorRef {
				// ignore DownMsg if it is not for our monitor
				continue
			}
			switch {
			case errors.Is(msg.Reason, exitreason.Kill):
				ck.parent <- childKillerDoneMsg{}
			case exitreason.IsShutdown(msg.Reason) && ck.child.Restart != Permanent:
				ck.parent <- childKillerDoneMsg{}
			case errors.Is(msg.Reason, exitreason.Normal) && ck.child.Restart != Permanent:
				ck.parent <- childKillerDoneMsg{}
			default:
				ck.parent <- childKillerDoneMsg{err: msg.Reason}
			}

		default:
			erl.DebugPrintf("childkiller[%s]: got a messsage that wasn't erl.DownMsg: %+v", ck.child.ID, msg)

		}
	}
}

func (ck *childKiller) handleTimeout(self erl.PID, inbox <-chan any) {
	erl.Exit(ck.parentPID, ck.child.pid, exitreason.SupervisorShutdown)
	for {
		select {
		case anyMsg, ok := <-inbox:
			if !ok {
				return
			}
			switch msg := anyMsg.(type) {
			case erl.DownMsg:
				ck.handleDown(self, msg)
				return

			default:
				erl.DebugPrintf("childkiller[%s]: got a messsage that wasn't erl.DownMsg: %+v", ck.child.ID, msg)

			}
		case <-time.After(chronos.Dur(fmt.Sprintf("%ds", ck.child.Shutdown.Timeout))):
			erl.Exit(ck.parentPID, ck.child.pid, exitreason.Kill)
			anyMsg := <-inbox
			switch msg := anyMsg.(type) {
			case erl.DownMsg:
				ck.handleDown(self, msg)

			default:
				erl.DebugPrintf("childkiller[%s]: got a messsage that wasn't erl.DownMsg: %+v", ck.child.ID, msg)
			}
		}
	}
}

func (ck *childKiller) handleDown(self erl.PID, msg erl.DownMsg) {
	if msg.Ref != ck.monitorRef {
		// ignore DownMsg if it is not for our monitor
		return
	}
	switch {
	case errors.Is(msg.Reason, exitreason.SupervisorShutdown):
		ck.parent <- childKillerDoneMsg{}
	case exitreason.IsShutdown(msg.Reason) && ck.child.Restart != Permanent:
		ck.parent <- childKillerDoneMsg{}
	case errors.Is(msg.Reason, exitreason.Normal) && ck.child.Restart != Permanent:
		ck.parent <- childKillerDoneMsg{}
	default:
		ck.parent <- childKillerDoneMsg{err: msg.Reason}
	}
}
