package supervisor

import (
	"errors"
	"fmt"

	"github.com/uberbrodt/erl-go/erl"
)

// Errors returned by dynamic child management APIs.
// Use [errors.Is] to check for specific error conditions.
var (
	// ErrNotFound is returned when a child ID does not exist in the supervisor.
	// Returned by [TerminateChild], [RestartChild], and [DeleteChild].
	ErrNotFound = errors.New("child not found")

	// ErrAlreadyPresent is returned by [StartChild] when a child with the given ID
	// already exists but is not running (terminated or ignored).
	// Use [RestartChild] to restart an existing terminated child, or [DeleteChild]
	// to remove the spec before adding a new one with the same ID.
	ErrAlreadyPresent = errors.New("child already present")

	// ErrAlreadyStarted is returned by [StartChild] when a child with the given ID
	// already exists and is currently running.
	// The actual error will be [AlreadyStartedError] which includes the existing PID.
	ErrAlreadyStarted = errors.New("child already started")

	// ErrRunning is returned when an operation cannot be performed because the
	// child is still running.
	// Returned by [DeleteChild] (must terminate first) and [RestartChild]
	// (child is already running).
	ErrRunning = errors.New("child is running")
)

// AlreadyStartedError is returned by [StartChild] when attempting to start a child
// with an ID that already exists and is running. It includes the PID of the
// existing running child.
//
// Use [errors.Is] with [ErrAlreadyStarted] to check for this condition:
//
//	_, err := supervisor.StartChild(sup, spec)
//	if errors.Is(err, supervisor.ErrAlreadyStarted) {
//		var asErr supervisor.AlreadyStartedError
//		if errors.As(err, &asErr) {
//			fmt.Printf("Child already running with PID: %v\n", asErr.PID)
//		}
//	}
type AlreadyStartedError struct {
	// PID is the process ID of the existing running child.
	PID erl.PID
}

// Error implements the error interface.
func (e AlreadyStartedError) Error() string {
	return fmt.Sprintf("child already started with PID %v", e.PID)
}

// Unwrap returns [ErrAlreadyStarted] so that [errors.Is] works correctly.
func (e AlreadyStartedError) Unwrap() error {
	return ErrAlreadyStarted
}
