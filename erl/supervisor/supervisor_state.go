package supervisor

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/uberbrodt/erl-go/chronos"
	"github.com/uberbrodt/erl-go/erl"
)

// childSpecs is an internal container for managing child specifications.
// It maintains children in start order and provides operations for
// lookup, update, and manipulation during supervisor operation.
type childSpecs struct {
	specs []ChildSpec
}

// get finds a child by ID and returns its index and spec.
// Returns error if no child with the given ID exists.
func (cs *childSpecs) get(childID string) (int, ChildSpec, error) {
	for idx, child := range cs.specs {
		if child.ID == childID {
			return idx, child, nil
		}
	}
	return 0, ChildSpec{}, fmt.Errorf("no child found by id: %v", childID)
}

// findByPID finds a child by its running process PID.
// Used when handling exit signals to identify which child terminated.
// Returns error if no child with the given PID exists.
func (cs *childSpecs) findByPID(pid erl.PID) (ChildSpec, error) {
	for _, childSpec := range cs.specs {
		if childSpec.pid.Equals(pid) {
			return childSpec, nil
		}
	}
	return ChildSpec{}, fmt.Errorf("no child matched pid: %v", pid)
}

// update replaces a child spec with the same ID.
// Used to update the PID after starting or restarting a child.
// Returns error if no child with the given ID exists.
func (cs *childSpecs) update(child ChildSpec) error {
	for idx, c := range cs.specs {
		if c.ID == child.ID {
			cs.specs[idx] = child
			return nil
		}
	}
	return fmt.Errorf("no child found by id: %v", child.ID)
}

// list returns all child specs in start order.
func (cs *childSpecs) list() []ChildSpec {
	return cs.specs
}

// delete removes a child by ID.
// Used when a Temporary child exits or a Transient child exits cleanly.
func (cs *childSpecs) delete(childID string) {
	newSpecs := slices.DeleteFunc(cs.specs, func(x ChildSpec) bool {
		return x.ID == childID
	})
	cs.specs = newSpecs
}

// split divides children into two groups at the given childID.
// The child with childID is included in the second group (right).
//
// Used by RestForOne strategy: when child B fails, split at B to get
// [A] and [B, C, D...], then restart [B, C, D...].
//
// Returns error if no child with the given ID exists.
func (cs *childSpecs) split(childID string) (*childSpecs, *childSpecs, error) {
	for idx, child := range cs.specs {
		if child.ID == childID {
			left := cs.specs[:idx]
			right := cs.specs[idx:]

			return &childSpecs{specs: left}, &childSpecs{specs: right}, nil
		}
	}
	return &childSpecs{}, &childSpecs{}, fmt.Errorf("could not split; no child id matched: %v", childID)
}

// append adds all children from another childSpecs to this one.
// Returns error if any duplicate IDs would result.
func (cs *childSpecs) append(in *childSpecs) error {
	cs.specs = append(cs.specs, in.specs...)

	return cs.checkDups()
}

// checkDups validates that all child IDs are unique and non-empty.
// Returns error describing the first validation failure found.
func (cs *childSpecs) checkDups() error {
	keyMap := make(map[string]struct{})

	var x struct{}
	for _, spec := range cs.specs {
		if strings.TrimSpace(spec.ID) == "" {
			return fmt.Errorf("childspec cannot have a blank id: %+v", spec)
		}
		_, ok := keyMap[spec.ID]
		if !ok {
			keyMap[spec.ID] = x
		} else {
			return fmt.Errorf("duplicate childspec id found: %s", spec.ID)
		}
	}
	return nil
}

// slice returns a subset of children by index range.
// Used internally for various list manipulations.
func (cs childSpecs) slice(startIn *int, endIn *int) childSpecs {
	var start int
	end := len(cs.specs) + 1
	if startIn != nil {
		start = *startIn
	}

	if endIn != nil {
		end = *endIn
	}
	return childSpecs{specs: cs.specs[start:end]}
}

// reverse returns a new childSpecs with children in reverse order.
// Used for shutdown (stop in reverse start order) and for OneForAll/RestForOne
// restart logic.
func (cs *childSpecs) reverse() *childSpecs {
	reversed := make([]ChildSpec, len(cs.specs))
	copy(reversed, cs.specs)
	slices.Reverse(reversed)
	return &childSpecs{specs: reversed}
}

// copy returns a shallow copy of the childSpecs.
func (cs *childSpecs) copy() childSpecs {
	cp := make([]ChildSpec, len(cs.specs))
	copy(cp, cs.specs)
	return childSpecs{specs: cp}
}

// newChildSpecs creates a childSpecs container and validates for duplicates.
// Returns error if any child IDs are duplicated or empty.
func newChildSpecs(specs []ChildSpec) (*childSpecs, error) {
	cs := &childSpecs{specs: specs}

	err := cs.checkDups()
	if err != nil {
		return cs, err
	}

	return cs, nil
}

// supervisorState holds the internal state of a running supervisor.
// This is the state type for the GenServer implementation.
type supervisorState struct {
	// args holds the arguments passed to StartLink (for potential future use)
	args any

	// callback is the Supervisor implementation providing Init configuration
	callback Supervisor

	// childSpecs is the original list of child specifications from Init
	childSpecs []ChildSpec

	// children is the current state of children (may differ from childSpecs
	// if Temporary children have been removed)
	children *childSpecs

	// flags contains the supervisor configuration (strategy, intensity, period)
	flags SupFlagsS

	// restarts tracks restart timestamps for intensity calculation.
	// Each entry is the time a restart occurred. Old entries (older than
	// Period seconds) are trimmed during addRestart.
	restarts []time.Time
}

// addRestart records a restart and checks if intensity is exceeded.
//
// The restart intensity algorithm:
//  1. Record current timestamp
//  2. Calculate the rolling window (now - Period seconds)
//  3. Count restarts within the window
//  4. If count > Intensity, return error (supervisor should terminate)
//  5. Trim old restarts outside the window to prevent unbounded growth
//
// Returns error if restart intensity is exceeded, signaling the supervisor
// to terminate itself and propagate failure up the supervision tree.
func (s supervisorState) addRestart() (supervisorState, error) {
	now := chronos.Now("")
	s.restarts = append(s.restarts, now)
	period := now.Add(-chronos.Dur(fmt.Sprintf("%ds", s.flags.Period)))
	var err error
	c := 0
	trim := 0
	for idx, r := range s.restarts {
		if r.After(period) {
			c += 1
			if c > s.flags.Intensity {
				erl.DebugPrintf("supervisor restart intensity exceeded")
				err = errors.New("supervisor restart intensity exceeded")
				break
			}
		} else {
			// if the item is not after the period, it doesn't count and needs to be trimmed
			trim = idx
		}
	}
	s.restarts = slices.Delete(s.restarts, 0, trim)
	return s, err
}
