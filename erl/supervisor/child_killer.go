package supervisor

import (
	"errors"
	"fmt"
	"time"

	"github.com/uberbrodt/erl-go/chronos"
	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
)

// childKillerDoneMsg is sent back to the supervisor when child termination completes.
// If err is non-nil, the child exited with an unexpected error (logged but not fatal).
type childKillerDoneMsg struct {
	err error
}

// childKiller is a helper process that handles terminating a single child.
//
// It's spawned by the supervisor to isolate the termination logic and handle
// the asynchronous nature of process shutdown. The childKiller:
//  1. Unlinks the child from the supervisor (to prevent exit signal)
//  2. Monitors the child (to receive DownMsg on termination)
//  3. Sends appropriate exit signal based on ShutdownOpt
//  4. Waits for DownMsg confirming termination
//  5. Reports completion back to supervisor via parent channel
//
// This design allows the supervisor to terminate multiple children concurrently
// if needed (though currently done sequentially) and cleanly handles timeouts.
type childKiller struct {
	// parent is the channel to send completion notification
	parent chan<- childKillerDoneMsg

	// parentPID is the supervisor's PID (used as sender for exit signals)
	parentPID erl.PID

	// child is the specification of the child being terminated
	child ChildSpec

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

// handleBrutalKill immediately kills the child without waiting for graceful shutdown.
//
// Sends exitreason.Kill which cannot be trapped and terminates the process
// immediately. Waits for DownMsg to confirm termination.
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

// handleTimeout sends shutdown signal and waits up to Timeout ms.
//
// If the child doesn't terminate within the timeout, sends Kill signal
// to force termination. This ensures the supervisor doesn't hang on
// misbehaving children.
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

// handleDown processes the DownMsg confirming child termination.
//
// Reports success (nil error) for expected exit reasons:
//   - SupervisorShutdown (requested shutdown)
//   - Shutdown (child-initiated clean shutdown, for non-Permanent children)
//   - Normal (clean exit, for non-Permanent children)
//
// Reports error for unexpected exit reasons (logged by supervisor).
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
