/*
erl programming system

the `erl` package wraps channels in a Process based system inspired by
Erlang and the Open Telecom Platform (OTP), which is the correct way to
build an Actor system on top of CSP.

For now all of OTP and the `gen` behaviours are left out, but the core
language primatives needed to build are implemented here:

  - process - wraps user code and responds to signals
  - monitor/demonitor signal - allows a process to be notified when the other
    process exits
  - link/unlink signal - joins two processes and causes them both to exit if
    either one does
  - Exit Trap - allows a process to ignore an exit signal and convert it
    to a message signal that is sent to the user code.

This all works, but the API here may change as we expand into OTP behaviours.
*/
package erl

import (
	"github.com/rs/xid"
)

type processStatus string

var (
	// starting processStatus = "starting"
	exiting processStatus = "EXITING"
	exited  processStatus = "EXITED"
	running processStatus = "RUNNING"
)

// A Process Identifier; wraps the underlying Process so we can reference it
// without exposing Process internals or provide a named process registry in the
// future
type PID struct {
	p *Process
}

// A Link is a bi-directional relationship between two processes. Once established,
// an exit signal in one process will always be sent to the other process. If
// you prefer to handle the [ExitMsg] in the [Runnable], then use [ProcessFlag] (TrapExit, true)
//
// To remove a link use [Unlink]
func Link(self PID, pid PID) {
	sendSignal(self, linkSignal{pid})
	sendSignal(pid, linkSignal{self})
}

// Removes a [Link] between two processes. No error is sent if the link does not exist.
func Unlink(self PID, pid PID) {
	sendSignal(self, unlinkSignal{pid})
	sendSignal(pid, unlinkSignal{self})
}

// Like [Spawn] but also creates a [Link] between the two processes.
func SpawnLink(self PID, r Runnable) PID {
	pid := Spawn(r)
	Link(self, pid)
	return pid
}

// This will establish a one-way relationship between [self] and [pid] and identified by the
// returned [Ref]. Once established, [self] will receive a [DownMsg] if the [pid] exits.
// Multiple Monitors between processes can be created (this is how synchronous communication
// between processes is handled)
func Monitor(self PID, pid PID) Ref {
	ref := MakeRef()
	signal := monitorSignal{ref: ref, monitor: self, monitored: pid}
	sendSignal(self, signal)
	sendSignal(pid, signal)
	return ref
}

// Removes a [Monitor]
func Demonitor(self PID, ref Ref) bool {
	sendSignal(self, demonitorSignal{ref: ref, origin: self})
	return true
}

// Like [Spawn] but also creates a [Monitor]
func SpawnMonitor(self PID, r Runnable) (PID, Ref) {
	pid := Spawn(r)
	ref := Monitor(self, pid)
	return pid, ref
}

// Creates a process and returns a [PID] that can be used to monitor
// or link to other process.
func Spawn(r Runnable) PID {
	p := NewProcess(r)

	go p.run()
	return PID{p: p}
}

func NewMsg(body any) Signal {
	return messageSignal{term: body}
}

// Sends the [term] to the process identified by [pid]. Will not error
// if process does not exist and will not block the caller.
func Send(pid PID, term any) {
	if pid.p.status == running {
		pid.p.receive <- messageSignal{term: term}
	}
}

func sendSignal(pid PID, signal Signal) {
	pid.p.receive <- signal
}

// Returns an opqaue "unique" identifier. Not crypto unique.
// Callers should not depend on size and structure of the returned
// [Ref]
func MakeRef() Ref {
	return Ref(xid.New().String())
}

// Returns true if the process is running
func IsAlive(pid PID) bool {
	return pid.p != nil && pid.p.status == running
}

func ProcessFlag(self PID, flag ProcFlag, value any) {
	if flag == TrapExit {
		v := value.(bool)

		self.p.trapExits = v
	}
}
