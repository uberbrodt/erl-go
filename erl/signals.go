package erl

import "github.com/uberbrodt/erl-go/erl/exitreason"

// A Signal is the low level communication method between processes. There are
// 7 signal types: link, unlink, monitor, demonitor, exit, down, and message.
// link/unlink and monitor/demonitor and used internally by the [process] code and are
// not exposed to Runnables directly. Runnables can only receive messageSignals, but
// the process will convert downSignals and exitSignals into [DownMsg] and [ExitMsg]
// so they can be consumed by [Runnable]s (only if the [TrapExit] flag is set on the
// process in the latter case).
type Signal interface {
	SignalName() string
}

type exitSignal struct {
	// PID of the process that sent the exit
	sender   PID
	receiver PID
	reason   *exitreason.S
	link     bool
}

func (s exitSignal) SignalName() string {
	return "exit"
}

// Received by a monitoring process when it's monitored process has exited.
// Will be forwarded to the Runable if a matching Ref is found, and discarded with
// a Warning log otherwise.
type downSignal struct {
	proc   PID
	ref    Ref
	reason *exitreason.S
}

func (s downSignal) SignalName() string {
	return "exit"
}

// MONITOR
type monitorSignal struct {
	ref       Ref
	monitor   PID
	monitored PID
}

func (s monitorSignal) SignalName() string {
	return "monitor"
}

// DEMONITOR
type demonitorSignal struct {
	ref Ref
	// used by the monitored process to make sure the demonitor call is coming from
	// the process that created the monitor in the first place.
	origin PID
}

func (s demonitorSignal) SignalName() string {
	return "demonitor"
}

// LINK
type linkSignal struct {
	pid PID
}

func (s linkSignal) SignalName() string {
	return "link"
}

// UNLINK
type unlinkSignal struct {
	pid PID
}

func (s unlinkSignal) SignalName() string {
	return "unlink"
}

// MESSAGE
type messageSignal struct {
	term any
}

func (s messageSignal) SignalName() string {
	return "msg"
}
