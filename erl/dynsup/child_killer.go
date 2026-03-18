package dynsup

import (
	"errors"
	"fmt"
	"time"

	"github.com/uberbrodt/erl-go/chronos"
	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
	"github.com/uberbrodt/erl-go/erl/supervisor"
)

// childKillerDoneMsg is sent back to the dynamic supervisor when child termination completes.
// If err is non-nil, the child exited with an unexpected error (logged but not fatal).
type childKillerDoneMsg struct {
	err error
}

// childKiller is a helper process that handles terminating a single child.
//
// It's spawned by the dynamic supervisor to isolate the termination logic and handle
// the asynchronous nature of process shutdown. The childKiller:
//  1. Unlinks the child from the supervisor (to prevent exit signal)
//  2. Monitors the child (to receive DownMsg on termination)
//  3. Sends appropriate exit signal based on ShutdownOpt
//  4. Waits for DownMsg confirming termination
//  5. Reports completion back to supervisor via parent channel
//
// Unlike the supervisor package's childKiller, this version takes the child PID
// as a direct parameter since supervisor.ChildSpec.pid is unexported.
type childKiller struct {
	// parent is the channel to send completion notification
	parent chan<- childKillerDoneMsg

	// parentPID is the dynamic supervisor's PID (used as sender for exit signals)
	parentPID erl.PID

	// childPID is the PID of the child being terminated
	childPID erl.PID

	// shutdown specifies how to terminate the child
	shutdown supervisor.ShutdownOpt

	// restart is the child's restart strategy (affects how exit reasons are interpreted)
	restart supervisor.Restart

	// monitorRef is the reference from monitoring the child
	monitorRef erl.Ref
}

// Receive implements [erl.Runnable] for the childKiller process.
//
// Handles termination based on the child's ShutdownOpt:
//   - BrutalKill: Immediately send Kill signal
//   - Infinity: Send SupervisorShutdown and wait forever
//   - Timeout: Send SupervisorShutdown, wait up to Timeout ms, then Kill
func (ck *childKiller) Receive(self erl.PID, inbox <-chan any) error {
	erl.DebugPrintf("DynSup %v is terminating child %v", ck.parentPID, ck.childPID)
	ck.monitorRef = erl.Monitor(self, ck.childPID)
	// unlink the supervisor so it doesn't get an ExitMsg
	erl.Unlink(ck.parentPID, ck.childPID)

	switch {
	case ck.shutdown.BrutalKill:
		ck.handleBrutalKill(self, inbox)
		return exitreason.Normal
	case ck.shutdown.Infinity:
		erl.Exit(self, ck.childPID, exitreason.SupervisorShutdown)
		for anyMsg := range inbox {
			switch msg := anyMsg.(type) {
			case erl.DownMsg:
				ck.handleDown(self, msg)
				return exitreason.Normal
			default:
				erl.DebugPrintf("dynsup childkiller: got a message that wasn't erl.DownMsg: %+v", msg)
			}
		}
	default:
		ck.handleTimeout(self, inbox)
		return exitreason.Normal
	}
	return exitreason.Normal
}

// handleBrutalKill immediately kills the child without waiting for graceful shutdown.
func (ck *childKiller) handleBrutalKill(self erl.PID, inbox <-chan any) {
	erl.Exit(ck.parentPID, ck.childPID, exitreason.Kill)
	for anyMsg := range inbox {
		switch msg := anyMsg.(type) {
		case erl.DownMsg:
			if msg.Ref != ck.monitorRef {
				continue
			}
			switch {
			case errors.Is(msg.Reason, exitreason.Kill):
				ck.parent <- childKillerDoneMsg{}
			case exitreason.IsShutdown(msg.Reason) && ck.restart != supervisor.Permanent:
				ck.parent <- childKillerDoneMsg{}
			case errors.Is(msg.Reason, exitreason.Normal) && ck.restart != supervisor.Permanent:
				ck.parent <- childKillerDoneMsg{}
			default:
				ck.parent <- childKillerDoneMsg{err: msg.Reason}
			}
		default:
			erl.DebugPrintf("dynsup childkiller: got a message that wasn't erl.DownMsg: %+v", msg)
		}
	}
}

// handleTimeout sends shutdown signal and waits up to Timeout ms.
func (ck *childKiller) handleTimeout(self erl.PID, inbox <-chan any) {
	erl.Exit(ck.parentPID, ck.childPID, exitreason.SupervisorShutdown)
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
				erl.DebugPrintf("dynsup childkiller: got a message that wasn't erl.DownMsg: %+v", msg)
			}
		case <-time.After(chronos.Dur(fmt.Sprintf("%dms", ck.shutdown.Timeout))):
			erl.Exit(ck.parentPID, ck.childPID, exitreason.Kill)
			anyMsg := <-inbox
			switch msg := anyMsg.(type) {
			case erl.DownMsg:
				ck.handleDown(self, msg)
			default:
				erl.DebugPrintf("dynsup childkiller: got a message that wasn't erl.DownMsg: %+v", msg)
			}
		}
	}
}

// handleDown processes the DownMsg confirming child termination.
func (ck *childKiller) handleDown(self erl.PID, msg erl.DownMsg) {
	if msg.Ref != ck.monitorRef {
		return
	}
	switch {
	case errors.Is(msg.Reason, exitreason.SupervisorShutdown):
		ck.parent <- childKillerDoneMsg{}
	case exitreason.IsShutdown(msg.Reason) && ck.restart != supervisor.Permanent:
		ck.parent <- childKillerDoneMsg{}
	case errors.Is(msg.Reason, exitreason.Normal) && ck.restart != supervisor.Permanent:
		ck.parent <- childKillerDoneMsg{}
	default:
		ck.parent <- childKillerDoneMsg{err: msg.Reason}
	}
}
