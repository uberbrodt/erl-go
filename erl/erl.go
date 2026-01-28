/*
Package erl provides Erlang-style process primitives for Go.

The erl package wraps goroutines in a Process-based system inspired by
Erlang and the Open Telecom Platform (OTP). This provides a foundation for
building concurrent, fault-tolerant applications using the Actor model on
top of Go's CSP primitives.

# Core Concepts

Processes in erl are lightweight wrappers around goroutines that provide:
  - Asynchronous message passing via [Send]
  - Process monitoring via [Monitor] and [SpawnMonitor]
  - Bi-directional linking via [Link] and [SpawnLink]
  - Exit signal handling and propagation
  - Automatic panic recovery with clean shutdown

# Process Creation

Use [Spawn], [SpawnLink], or [SpawnMonitor] to create new processes:

	// Standalone process
	pid := erl.Spawn(myRunnable)

	// Linked to parent (exit signals propagate both ways)
	pid := erl.SpawnLink(self, myRunnable)

	// Monitored by parent (one-way notification on exit)
	pid, ref := erl.SpawnMonitor(self, myRunnable)

# Links vs Monitors

Links ([Link], [SpawnLink]) create a bi-directional relationship. When either
process exits, the other receives an exit signal and terminates (unless
trapping exits). Use links for processes that should live and die together,
such as a worker and its supervisor.

Monitors ([Monitor], [SpawnMonitor]) create a one-way relationship. The
monitoring process receives a [DownMsg] when the monitored process exits,
but is not affected itself. Use monitors when you need to know about process
death without being killed.

# Exit Trapping

By default, when a linked process exits with a non-normal reason, the exit
signal kills the receiving process. Use [ProcessFlag] with [TrapExit] to
convert exit signals into [ExitMsg] messages that can be handled:

	erl.ProcessFlag(self, erl.TrapExit, true)

This is essential for supervisors and any process that needs to handle
linked process failures gracefully.

# Panic Recovery

All user code running in a Process is automatically protected by panic recovery.
When a panic occurs in a Runnable's Receive method (including GenServer callbacks),
the Process wrapper catches it, converts it to [exitreason.Exception], and exits
cleanly. This ensures:

  - Linked processes and monitors are properly notified
  - Supervisors can restart the process from clean state
  - No goroutine leaks or undefined behavior

You do not need to add defer/recover in your Runnable implementations. The
Process-level panic handler provides universal protection.

# Erlang Correspondence

This package implements Go equivalents of Erlang's core process primitives:

	Erlang                  Go (erl package)
	------                  ----------------
	spawn/1                 Spawn
	spawn_link/1            SpawnLink
	spawn_monitor/1         SpawnMonitor
	link/1                  Link
	unlink/1                Unlink
	monitor/2               Monitor
	demonitor/1             Demonitor
	!/send                  Send
	exit/2                  Exit
	process_flag/2          ProcessFlag
	is_process_alive/1      IsAlive
	make_ref/0              MakeRef
	send_after/3            SendAfter

See https://www.erlang.org/doc/system/ref_man_processes.html for detailed
Erlang process documentation that informed this design.
*/
package erl

import (
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

// Link establishes a bi-directional relationship between two processes.
//
// This is equivalent to Erlang's link/1. Once linked, if either process exits,
// the other receives an exit signal. By default, this causes the receiving
// process to exit with the same reason (exit signal propagation).
//
// To handle exit signals instead of dying, use [ProcessFlag] to set [TrapExit]
// to true. The process will then receive [ExitMsg] messages that can be
// handled in the [Runnable.Receive] method.
//
// Parameters:
//   - self: The PID of the calling process
//   - pid: The PID of the process to link with
//
// Links are idempotent: calling Link multiple times between the same processes
// has no additional effect. If pid refers to a dead or non-existent process,
// self immediately receives an exit signal with reason [exitreason.NoProc].
//
// To remove a link, use [Unlink]. Links are automatically removed when either
// process exits.
//
// Example:
//
//	// Link two processes
//	erl.Link(self, workerPID)
//
//	// To handle linked process exits instead of dying:
//	erl.ProcessFlag(self, erl.TrapExit, true)
func Link(self PID, pid PID) {
	sendSignal(self, linkSignal{pid})
	sendSignal(pid, linkSignal{self})
}

// Unlink removes a link between two processes.
//
// This is equivalent to Erlang's unlink/1. After unlinking, exit signals
// will no longer propagate between the two processes. This operation is
// idempotent: calling Unlink when no link exists has no effect and does
// not return an error.
//
// Parameters:
//   - self: The PID of the calling process
//   - pid: The PID of the process to unlink from
//
// Note: Unlink does not prevent exit signals that are already in flight.
// If the other process has already exited, the exit signal may still be
// delivered even after Unlink is called.
func Unlink(self PID, pid PID) {
	sendSignal(self, unlinkSignal{pid})
	sendSignal(pid, unlinkSignal{self})
}

// SpawnLink creates a new process and atomically links it to the caller.
//
// This is equivalent to Erlang's spawn_link/1. The new process is created
// and linked in a single atomic operation, ensuring no race condition where
// the child could exit before the link is established.
//
// Parameters:
//   - self: The PID of the calling process (will be linked to the new process)
//   - r: The [Runnable] implementation that defines the process behavior
//
// Returns the PID of the newly spawned process.
//
// This is the recommended way to start worker processes under a supervisor,
// as it ensures exit signals propagate correctly for fault tolerance.
// If the spawned process exits abnormally, self will receive an exit signal
// (or [ExitMsg] if trapping exits).
//
// Example:
//
//	worker := erl.SpawnLink(self, &MyWorker{config: cfg})
func SpawnLink(self PID, r Runnable) PID {
	pid := doSpawn(r, nil, &self)
	return pid
}

// Monitor establishes a one-way observation relationship between processes.
//
// This is equivalent to Erlang's monitor(process, Pid). Unlike [Link], monitoring
// is unidirectional: the monitoring process (self) receives a [DownMsg] when the
// monitored process (pid) exits, but the monitored process is unaware of and
// unaffected by the monitor.
//
// Parameters:
//   - self: The PID of the monitoring process (receives [DownMsg] on exit)
//   - pid: The PID of the process to monitor
//
// Returns a [Ref] that uniquely identifies this monitor. Use this reference
// with [Demonitor] to remove the monitor.
//
// Multiple monitors can exist between the same pair of processes. Each monitor
// has a unique [Ref] and generates a separate [DownMsg] when the monitored
// process exits. This property is used internally for implementing synchronous
// calls (gen_caller).
//
// If pid refers to a dead or non-existent process, self immediately receives
// a [DownMsg] with reason [exitreason.NoProc].
//
// Example:
//
//	ref := erl.Monitor(self, workerPID)
//	// Later, in Receive:
//	// case erl.DownMsg: // workerPID has exited
func Monitor(self PID, pid PID) Ref {
	ref := MakeRef()
	signal := monitorSignal{ref: ref, monitor: self, monitored: pid}
	sendSignal(self, signal)
	sendSignal(pid, signal)
	return ref
}

// Demonitor removes a previously established monitor.
//
// This is equivalent to Erlang's demonitor/1. After calling Demonitor, no
// [DownMsg] will be delivered for the specified monitor reference, even if
// the monitored process subsequently exits.
//
// Parameters:
//   - self: The PID of the monitoring process
//   - ref: The [Ref] returned by [Monitor] or [SpawnMonitor]
//
// Returns true. This function always succeeds, even if the monitor doesn't
// exist (idempotent operation).
//
// Note: Like Erlang's demonitor/1 without the flush option, this does not
// remove any [DownMsg] that may already be in the process inbox. If the
// monitored process has already exited, the [DownMsg] may still be received.
func Demonitor(self PID, ref Ref) bool {
	sendSignal(self, demonitorSignal{ref: ref, origin: self})
	return true
}

// SpawnMonitor creates a new process and atomically monitors it from the caller.
//
// This is equivalent to Erlang's spawn_monitor/1. The new process is created
// and monitored in a single atomic operation, ensuring no race condition where
// the child could exit before the monitor is established.
//
// Parameters:
//   - self: The PID of the calling process (will monitor the new process)
//   - r: The [Runnable] implementation that defines the process behavior
//
// Returns the PID of the newly spawned process and a [Ref] identifying the
// monitor. Use the Ref with [Demonitor] to remove the monitor.
//
// Unlike [SpawnLink], the caller is not affected if the spawned process exits.
// Instead, the caller receives a [DownMsg] msg in its inbox. This is useful
// when you need to track process lifecycle without risking your own termination.
//
// Example:
//
//	pid, ref := erl.SpawnMonitor(self, &MyWorker{})
//	// Later, in Receive:
//	// case erl.DownMsg: // worker has exited, can spawn replacement
func SpawnMonitor(self PID, r Runnable) (PID, Ref) {
	ref := MakeRef()
	pid := doSpawn(r, &spawnMonitor{pid: self, ref: ref}, nil)

	// ref := Monitor(self, pid)
	return pid, ref
}

// doSpawn is the internal implementation for Spawn, SpawnLink, and SpawnMonitor.
// It creates a new Process, optionally sets up monitor/link relationships, and
// starts the process goroutine.
func doSpawn(r Runnable, sm *spawnMonitor, link *PID) PID {
	p := NewProcess(r)
	p.spawnMonitor = sm
	p.spawnLink = link

	go p.run()
	return PID{p: p}
}

// Spawn creates a new process and returns its PID.
//
// This is equivalent to Erlang's spawn/1. The new process runs independently
// of the caller with no link or monitor relationship. Neither process is
// notified if the other exits.
//
// Parameters:
//   - r: The [Runnable] implementation that defines the process behavior
//
// Returns the PID of the newly spawned process. The PID can be used to:
//   - Send messages via [Send]
//   - Establish links via [Link]
//   - Establish monitors via [Monitor]
//   - Send exit signals via [Exit]
//
// For processes that should be supervised, use [SpawnLink] instead to ensure
// proper exit signal propagation. Use Spawn only for truly independent
// processes where you don't care about their lifecycle.
//
// Example:
//
//	pid := erl.Spawn(&MyProcess{})
//	erl.Send(pid, MyMessage{data: "hello"})
func Spawn(r Runnable) PID {
	return doSpawn(r, nil, nil)
}

// NewMsg wraps a value in a message signal for internal use.
//
// This is primarily used internally for creating message signals. Most users
// should use [Send] instead, which handles signal creation automatically.
func NewMsg(body any) Signal {
	return messageSignal{term: body}
}

// Send delivers a message to a process asynchronously.
//
// This is equivalent to Erlang's ! (send) operator. The message is placed
// in the target process's inbox and will be delivered to its [Runnable.Receive]
// method. Send is completely asynchronous: it returns immediately without
// waiting for the message to be processed or acknowledged.
//
// Parameters:
//   - pid: The PID of the target process
//   - term: The message to send (any Go value)
//
// Send never blocks and never returns an error. If the target process doesn't
// exist or has already exited, the message is silently discarded. This is
// consistent with Erlang's fire-and-forget semantics for message passing.
//
// For synchronous request/response patterns, use genservers and [genserver.Call]
// instead. For messages that require delivery confirmation, consider using
// a monitor or implementing an application-level acknowledgment protocol.
//
// Example:
//
//	erl.Send(workerPID, WorkRequest{id: 123, data: payload})
func Send(pid PID, term any) {
	sendSignal(pid, messageSignal{term: term})
}

// SendAfter schedules a message to be sent to a process after a delay.
//
// This is equivalent to Erlang's erlang:send_after/3. After the specified
// duration elapses, the message is delivered to the target process as if
// [Send] had been called.
//
// Parameters:
//   - pid: The PID of the target process
//   - term: The message to send after the delay
//   - tout: The delay before sending the message
//
// Returns a [TimerRef] that can be used to cancel the pending send with
// [CancelTimer]. If the target process doesn't exist or is not running,
// returns an empty TimerRef and no message is scheduled.
//
// The timer is implemented as a separate process that sleeps for the
// specified duration and then sends the message. This means:
//   - The message delivery is best-effort (target may exit before delivery)
//   - Timer precision depends on Go's time.Sleep accuracy
//   - Each SendAfter spawns a lightweight process
//
// Example:
//
//	// Send a timeout message after 5 seconds
//	ref := erl.SendAfter(self, TimeoutMsg{}, 5*time.Second)
//
//	// Cancel if no longer needed
//	erl.CancelTimer(ref)
func SendAfter(pid PID, term any, tout time.Duration) TimerRef {
	if pid != UndefinedPID && pid.p.getStatus() == running {
		t := &timer{to: pid, term: term, tout: tout}

		timerPid := Spawn(t)

		return TimerRef{pid: timerPid}
	}
	return TimerRef{}
}

// sendSignal is the internal function for delivering signals to processes.
//
// If the target process is alive, the signal is delivered to its inbox.
// If the target is dead or undefined, appropriate responses are sent:
//   - linkSignal: sender receives exitSignal with NoProc reason
//   - monitorSignal: monitor receives downSignal with NoProc reason
//   - Other signals: silently discarded
func sendSignal(pid PID, signal Signal) {
	// if process isn't alive, pid.p may be nil
	if pid != UndefinedPID && !pid.IsNil() && pid.p.getStatus() == running {
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

// MakeRef generates a new unique reference.
//
// This is equivalent to Erlang's make_ref/0. References are opaque identifiers
// used to uniquely tag requests, monitors, and other operations that need
// correlation. Each call to MakeRef returns a globally unique value.
//
// Returns a [Ref] that is unique within the lifetime of the program. The
// reference is not cryptographically secure and should not be used for
// security-sensitive operations. Callers should not depend on the internal
// structure or size of the returned Ref.
//
// Common uses:
//   - Correlating request/response pairs
//   - Identifying specific monitors (returned by [Monitor] and [SpawnMonitor])
//   - Creating unique message tags
func MakeRef() Ref {
	return Ref(xid.New().String())
}

// UndefinedRef is the zero value for [Ref], representing no reference.
var UndefinedRef Ref = Ref("")

// IsAlive checks if a process is currently running.
//
// This is equivalent to Erlang's is_process_alive/1. Returns true if the
// process exists and has not yet exited. Note that this is a point-in-time
// check: the process could exit immediately after IsAlive returns true.
//
// Parameters:
//   - pid: The PID to check
//
// Returns false if:
//   - The PID is nil or undefined
//   - The process has exited
//   - The process was never started
//
// For reliable process lifecycle tracking, prefer using [Monitor] instead
// of polling IsAlive, as monitors provide guaranteed notification of exit.
func IsAlive(pid PID) bool {
	return !pid.IsNil() && pid.p.getStatus() == running
}

// ProcessFlag sets process configuration flags that modify behavior.
//
// This is equivalent to Erlang's process_flag/2. Currently the only supported
// flag is [TrapExit], which controls how exit signals are handled.
//
// Parameters:
//   - self: The PID of the process to configure (must be the calling process)
//   - flag: The flag to set (see [ProcFlag])
//   - value: The value for the flag (type depends on flag)
//
// Supported flags:
//
//	TrapExit (bool): When true, exit signals from linked processes are
//	converted to [ExitMsg] messages instead of killing the process. This
//	is essential for supervisors and any process that needs to handle
//	linked process failures.
//
// Panics if self is nil.
//
// Example:
//
//	// Enable exit trapping to handle linked process failures
//	erl.ProcessFlag(self, erl.TrapExit, true)
//
//	// In Receive, you can now handle ExitMsg:
//	// case erl.ExitMsg:
//	//     log.Printf("linked process %v exited: %v", msg.Proc, msg.Reason)
func ProcessFlag(self PID, flag ProcFlag, value any) {
	if self.IsNil() {
		panic("pid cannot be nil")
	}
	if flag == TrapExit {
		v := value.(bool)

		self.p.setTrapExits(v)
	}
}

// TrappingExits reports whether a process has exit trapping enabled.
//
// Returns true if [ProcessFlag] was called with [TrapExit] set to true for
// the given process. When exit trapping is enabled, the process receives
// [ExitMsg] messages instead of being killed by exit signals from linked
// processes.
//
// Parameters:
//   - self: The PID of the process to check
//
// Returns false if self is nil or if the process is not trapping exits.
func TrappingExits(self PID) bool {
	if self.IsNil() {
		return false
	}

	return self.p.trapExits()
}

// Exit sends an exit signal to a process.
//
// This is equivalent to Erlang's exit/2. The exit signal is sent from self
// to pid with the specified reason. How the signal is handled depends on
// whether the target process is trapping exits and the exit reason:
//
// If pid is trapping exits ([TrapExit] is true):
//   - The exit signal is converted to an [ExitMsg] delivered to Receive
//   - Exception: reason [exitreason.Kill] cannot be trapped and always kills
//
// If pid is NOT trapping exits:
//   - reason [exitreason.Normal]: Signal is ignored (process continues)
//   - reason [exitreason.Kill]: Process is killed unconditionally
//   - Any other reason: Process exits with that reason
//
// Parameters:
//   - self: The PID of the sending process
//   - pid: The PID of the process to signal
//   - reason: The exit reason (see [exitreason] package)
//
// Unlike [Send], Exit can affect the target process's lifecycle. Use this
// for explicit process termination or shutdown coordination.
//
// Example:
//
//	// Request graceful shutdown
//	erl.Exit(self, workerPID, exitreason.Shutdown(nil))
//
//	// Force kill (cannot be trapped)
//	erl.Exit(self, hungPID, exitreason.Kill)
func Exit(self PID, pid PID, reason *exitreason.S) {
	es := exitSignal{sender: self, receiver: pid, reason: reason}

	pid.p.send(es)
}
