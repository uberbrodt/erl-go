package erl

import "github.com/uberbrodt/erl-go/erl/exitreason"

// an opaque unique string. Don't rely on structure format or even size for that matter.
type Ref string

// A Process managed
type Runnable interface {
	Receive(self PID, inbox <-chan any) error
}

type ProcFlag string

var TrapExit ProcFlag = "trap_exit"

type ExitMsg struct {
	// the process that sent the exit signal
	Proc   PID
	Reason *exitreason.S
	// whether the exit was caused by a link. If false, then this was sent via [Exit()]
	Link bool
}

func exitMsgFromSignal(es exitSignal) ExitMsg {
	return ExitMsg{Proc: es.sender, Reason: es.reason, Link: es.link}
}

type DownMsg struct {
	Proc   PID
	Reason *exitreason.S
	Ref    Ref
}

func downMsgfromSignal(es downSignal) DownMsg {
	return DownMsg{Proc: es.proc, Reason: es.reason, Ref: es.ref}
}
