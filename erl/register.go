package erl

import (
	"sync"
)

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

func WhereIs(name Name) (pid PID, exists bool) {
	registrationMutex.RLock()
	defer registrationMutex.RUnlock()

	pid, exists = names[name]
	return
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

func Registered() []Registration {
	registrations := make([]Registration, 0, len(names))
	for name, pid := range names {
		registrations = append(registrations, Registration{Name: name, PID: pid})
	}
	return registrations
}
