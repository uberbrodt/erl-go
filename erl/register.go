package erl

import (
	"sync"
)

// Process name registry
//
// The registry provides a global namespace for associating Names with PIDs.
// All registry operations are thread-safe and can be called from any goroutine.
//
// Processes automatically unregister themselves when they exit, so stale
// entries do not accumulate. However, there is a brief window during process
// shutdown where a name may still be registered but IsAlive returns false.

var (
	names             map[Name]PID
	registrationMutex sync.RWMutex
)

func init() {
	names = make(map[Name]PID)
}

type RegistrationErrorKind string

type RegistrationError struct {
	Kind RegistrationErrorKind
}

func (e *RegistrationError) Error() string {
	return string(e.Kind)
}

const (
	// process is already registered with a name. Caller should consider calling [Unregister] and retry
	AlreadyRegistered RegistrationErrorKind = "already_registered"
	// another process already registered given name
	NameInUse RegistrationErrorKind = "name_used"
	// the process you're trying to register doesn't exist/is dead
	NoProc RegistrationErrorKind = "no_proc"
	// name is invalid and cannot be registered.
	BadName RegistrationErrorKind = "bad_name"
)

// Register associates the given name with the given PID in the global registry.
// The name must be unique and the process must be alive.
//
// Returns nil on success, or a *RegistrationError indicating the failure reason:
//   - [BadName]: name is empty or reserved ("nil", "undefined")
//   - [NoProc]: the process is not alive
//   - [AlreadyRegistered]: the process already has a registered name
//   - [NameInUse]: another process is already registered with this name
//
// A process can only have one registered name at a time. To change a process's
// name, first call [Unregister] with the old name.
func Register(name Name, pid PID) *RegistrationError {
	registrationMutex.Lock()
	defer registrationMutex.Unlock()
	if name == "nil" || name == "undefined" || name == "" {
		return &RegistrationError{Kind: BadName}
	}

	if !IsAlive(pid) {
		return &RegistrationError{Kind: NoProc}
	}

	if pid.p.getName() != "" {
		return &RegistrationError{Kind: AlreadyRegistered}
	}

	if _, ok := names[name]; ok {
		return &RegistrationError{Kind: NameInUse}
	}

	names[name] = pid
	// make sure process knows it's name. It will know to unregister itself
	// when it exits now.
	pid.p.setName(name)
	return nil
}

// WhereIs looks up the PID registered under the given name.
// Returns the PID and true if found, or UndefinedPID and false if not registered.
//
// Note: The returned PID may refer to a process that is in the process of
// exiting. Use [IsAlive] to verify the process is still running if needed.
func WhereIs(name Name) (pid PID, exists bool) {
	registrationMutex.RLock()
	defer registrationMutex.RUnlock()

	pid, exists = names[name]
	return pid, exists
}

// Unregister given [Name]. Returns false if [Name] is not registered
func Unregister(name Name) bool {
	registrationMutex.Lock()
	defer registrationMutex.Unlock()
	if pid, ok := names[name]; ok {
		delete(names, name)
		// NOTE: need to unset the process name, otherwise it cannot be re-registered
		pid.p.setName(Name(""))
		return true
	}
	return false
}

type Registration struct {
	Name Name
	PID  PID
}

// Registered returns a snapshot of all currently registered name-PID pairs.
// The returned slice is safe to use after the call returns, but the registrations
// themselves may become stale if processes exit or names are unregistered.
func Registered() []Registration {
	registrationMutex.RLock()
	defer registrationMutex.RUnlock()

	registrations := make([]Registration, 0, len(names))
	for name, pid := range names {
		registrations = append(registrations, Registration{Name: name, PID: pid})
	}
	return registrations
}

// RegisteredCount returns the number of currently registered names.
// This is more efficient than len(Registered()) when you only need the count.
func RegisteredCount() int {
	registrationMutex.RLock()
	defer registrationMutex.RUnlock()
	return len(names)
}
