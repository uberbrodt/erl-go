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
	"fmt"
	"time"

	"github.com/rs/xid"

	"github.com/uberbrodt/erl-go/erl/exitreason"
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

var UndefinedPID PID = PID{}

func (pid PID) String() string {
	if pid.p != nil {
		if pid.p.getName() == "" {
			return fmt.Sprintf("PID<%d>", pid.p.id)
		} else {
			return fmt.Sprintf("PID<%d|%s>", pid.p.id, pid.p.getName())
		}
	} else {
		return "PID<undefined>"
	}
}

func (pid PID) IsNil() bool {
	return pid.p == nil
}

func (self PID) Equals(pid PID) bool {
	if self.IsNil() && pid.IsNil() {
		return true
	}

	if self.IsNil() || pid.IsNil() {
		return false
	}

	return self.p.id == pid.p.id
}

// TODO: might want to return an error if p.status != running?
func (p PID) ResolvePID() (PID, error) {
	return p, nil
}

type PIDIface interface {
	isPID() bool
	isRootPID() bool
}

type Name string

func (n Name) ResolvePID() (PID, error) {
	pid, exists := WhereIs(n)
	if !exists {
		return pid, fmt.Errorf("no PID found for name %s", n)
	}
	return pid, nil
}

type Dest interface {
	ResolvePID() (PID, error)
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
	sendSignal(pid, messageSignal{term: term})
}

func SendAfter(pid PID, term any, tout time.Duration) TimerRef {
	if pid != UndefinedPID && pid.p.getStatus() == running {
		t := &timer{to: pid, term: term, tout: tout}

		timerPid := Spawn(t)

		return TimerRef{pid: timerPid}
	}
	return TimerRef{}
}

func sendSignal(pid PID, signal Signal) {
	// if process isn't alive, pid.p may be nil
	if pid != UndefinedPID && !pid.IsNil() {
		pid.p.send(signal)
	} else {
		// if the process is dead we reply with exit/down msgs as needed. All other messages are ignored
		switch sig := signal.(type) {
		case linkSignal:
			sig.pid.p.send(exitSignal{sender: pid, receiver: sig.pid, reason: exitreason.NoProc, link: true})
		case monitorSignal:
			sig.monitor.p.send(downSignal{proc: pid, ref: sig.ref, reason: exitreason.NoProc})
		default:
			// just ignore

		}
	}
}

// Returns an opqaue "unique" identifier. Not crypto unique.
// Callers should not depend on size and structure of the returned
// [Ref]
func MakeRef() Ref {
	return Ref(xid.New().String())
}

var UndefinedRef Ref = Ref("")

// Returns true if the process is running
func IsAlive(pid PID) bool {
	return !pid.IsNil() && pid.p.getStatus() == running
}

func ProcessFlag(self PID, flag ProcFlag, value any) {
	if self.IsNil() {
		panic("pid cannot be nil")
	}
	if flag == TrapExit {
		v := value.(bool)

		self.p.setTrapExits(v)
	}
}

func Exit(self PID, pid PID, reason *exitreason.S) {
	es := exitSignal{sender: self, receiver: pid, reason: reason}

	pid.p.send(es)
}
