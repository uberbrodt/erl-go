package supervisor

import (
	"errors"
	"fmt"
	"time"

	"golang.org/x/exp/slices"

	"github.com/uberbrodt/erl-go/chronos"
	"github.com/uberbrodt/erl-go/erl"
)

type childSpecs struct {
	specs []ChildSpec
}

func (cs *childSpecs) get(childID string) (int, ChildSpec, error) {
	for idx, child := range cs.specs {
		if child.ID == childID {
			return idx, child, nil
		}
	}
	return 0, ChildSpec{}, fmt.Errorf("no child found by id: %v", childID)
}

func (cs *childSpecs) findByPID(pid erl.PID) (ChildSpec, error) {
	for _, childSpec := range cs.specs {
		if childSpec.pid.Equals(pid) {
			return childSpec, nil
		}
	}
	return ChildSpec{}, fmt.Errorf("no child matched pid: %v", pid)
}

func (cs *childSpecs) update(child ChildSpec) error {
	for idx, c := range cs.specs {
		if c.ID == child.ID {
			cs.specs[idx] = child
			return nil
		}
	}
	return fmt.Errorf("no child found by id: %v", child.ID)
}

func (cs *childSpecs) list() []ChildSpec {
	return cs.specs
}

func (cs *childSpecs) delete(childID string) {
	newSpecs := slices.DeleteFunc(cs.specs, func(x ChildSpec) bool {
		return x.ID == childID
	})
	cs.specs = newSpecs
}

// [childID] will be  included as the start of the second *childSpecs
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

func (cs *childSpecs) append(in *childSpecs) error {
	cs.specs = append(cs.specs, in.specs...)

	return cs.checkDups()
}

func (cs *childSpecs) checkDups() error {
	keyMap := make(map[string]struct{})

	var x struct{}
	for _, spec := range cs.specs {
		_, ok := keyMap[spec.ID]
		if !ok {
			keyMap[spec.ID] = x
		} else {
			return fmt.Errorf("duplicate childspec id found: %s", spec.ID)
		}
	}
	return nil
}

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

func (cs *childSpecs) reverse() *childSpecs {
	reversed := make([]ChildSpec, len(cs.specs))
	copy(reversed, cs.specs)
	slices.Reverse(reversed)
	return &childSpecs{specs: reversed}
}

func (cs *childSpecs) copy() childSpecs {
	cp := make([]ChildSpec, len(cs.specs))
	copy(cp, cs.specs)
	return childSpecs{specs: cp}
}

func newChildSpecs(specs []ChildSpec) (*childSpecs, error) {
	cs := &childSpecs{specs: specs}

	err := cs.checkDups()
	if err != nil {
		return cs, err
	}

	return cs, nil
}

type supervisorState struct {
	args       any
	callback   Supervisor
	childSpecs []ChildSpec
	children   *childSpecs

	flags    SupFlagsS
	restarts []time.Time
}

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
