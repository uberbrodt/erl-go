package erl

import "fmt"

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
