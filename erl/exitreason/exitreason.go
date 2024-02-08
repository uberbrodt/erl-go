// Each process should return a valid [exitreason.S] when it's [Runable.Receive] returns.
// The specific kind of return will influence how higher order components such as Supervisors
// and Registries handle process exits.
//
// In the event a process exits unexpectedly (ie. panic), then an Exception reason is returned.
package exitreason

import (
	"errors"
	"fmt"
)

const (
	normal             = "normal"
	shutdown           = "shutdown"
	supervisorShutdown = "supervisor_shutdown"
	exception          = "error"
	noProc             = "noproc"
	timeout            = "timeout"
	kill               = "kill"
	brutalKill         = "brutal_kill"
	ignore             = "ignore"
	stopped            = "stopped"
	// sent to testReceiver to cause it to stop
	testExit = "test_exit"
	// for empty reasons
	none = "none"
)

// Opaque return type. Use functions in this package to create new instances and
// test with the `Is*(exitreason) bool` functions.
//
// For convienence it implements the [errors] and [stringer] interfaces.
type S struct {
	short          string
	err            error
	shutdownReason any
	exception      error
}

func (s *S) Error() string {
	switch {
	case s.short == exception:
		return fmt.Sprintf("EXIT{error: %v}", s.exception)
	case s.short == shutdown:
		return fmt.Sprintf("EXIT{shutdown: %v}", s.shutdownReason)
	default:
		return fmt.Sprintf("EXIT{%s}", s.short)
	}
}

// Shutdown exitreasons include optional information
func (s *S) ShutdownReason() any {
	return s.shutdownReason
}

func (s *S) ExceptionDetail() error {
	return s.exception
}

// func (s *S) Wrap() error { return s.err }

func (s *S) Unwrap() error {
	return s.err
}

// private Sentinel Errors that get wrapped
var (
	shutdownErr  = &S{short: shutdown}
	exceptionErr = &S{short: exception}
)

// Sentinel errors
var (
	// A normal process exit.
	Normal = &S{short: normal}
	// Indicates a process's Supervisor terminated the process. Also considered a "Normal" exit.
	SupervisorShutdown = &S{short: supervisorShutdown}
	// The pid or name does not identifiy an active process
	NoProc = &S{short: noProc}
	// Returned when a request exceeds it's specified timeout.
	Timeout = &S{short: timeout}
	// This is special type of exit reason that would allow a Supervisor to ignore
	// a genserver that doesn't start while keeping it's child specification around
	// so it can be started later.
	Ignore = &S{short: ignore}
	// Untrappable exit signal.
	Kill = &S{short: kill}
	// The process stopped after replying to a Call
	Stopped  = &S{short: stopped}
	TestExit = &S{short: testExit}
)

// Tests to see if error is or wraps a *S. If not, returns nil
func IsExitReason(e error) (err *S) {
	ok := errors.As(e, &err)

	if ok {
		return err
	}

	return nil
}

// Test if [exitReason] is "Normal"
func IsNormal(e error) bool {
	return errors.Is(e, Normal)
}

// Returned when a process is exiting cleanly. Optionally provide additional info that will
// be returned to monitors/links. Considered a "Normal" exit.
func Shutdown(reason any) error {
	return &S{short: shutdown, shutdownReason: reason, err: shutdownErr}
}

// Test if [exitReason] is "Shutdown"
func IsShutdown(e error) bool {
	return errors.Is(e, shutdownErr)
}

// General "error" exitreason. Returned when a process panicks or exits with an error.
func Exception(reason error) error {
	return &S{exception: reason, err: exceptionErr, short: exception}
}

// Test if [exitReason] is "Exception"
func IsException(e error) bool {
	return errors.Is(e, exceptionErr)
}

func To(e error) *S {
	return IsExitReason(e)
}

// Takes any error and if it is not a *S, then wraps it as an [exitreason.Exception]
func Wrap(e error) error {
	if er := IsExitReason(e); er != nil {
		return er
	} else {
		return Exception(e)
	}
}
