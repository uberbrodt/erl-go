package erl

// an opaque unique string. Don't rely on structure format or even size for that matter.
type Ref string

// A Process managed
type Runnable interface {
	Receive(self PID, inbox <-chan any) error
}

type ProcFlag string

var TrapExit ProcFlag = "trap_exit"

type (
	ExitMsg = exitSignal
	DownMsg = downSignal
)
