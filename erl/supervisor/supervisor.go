package supervisor

import (
	"errors"
	"fmt"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
	"github.com/uberbrodt/erl-go/erl/genserver"
)

var errRestartsExceeded = errors.New("supervisor restart intensity exceeded")

var _ genserver.GenServer[supervisorState] = SupervisorS{}

type SupFlagsS struct {
	Strategy Strategy
	// duration of the evaluation window for restarts, in seconds
	Period int
	// how many restarts allowed within [Period]
	Intensity int
}

type SupFlag func(flags SupFlagsS) SupFlagsS

func SetStrategy(strategy Strategy) SupFlag {
	return func(flags SupFlagsS) SupFlagsS {
		flags.Strategy = strategy
		return flags
	}
}

func SetPeriod(period int) SupFlag {
	return func(flags SupFlagsS) SupFlagsS {
		flags.Period = period
		return flags
	}
}

func SetIntensity(intensity int) SupFlag {
	return func(flags SupFlagsS) SupFlagsS {
		flags.Intensity = intensity
		return flags
	}
}

func NewSupFlags(flags ...SupFlag) SupFlagsS {
	f := SupFlagsS{
		Strategy:  OneForOne,
		Period:    5,
		Intensity: 1,
	}

	for _, x := range flags {
		f = x(f)
	}
	return f
}

type InitResult struct {
	SupFlags   SupFlagsS
	ChildSpecs []ChildSpec
	Ignore     bool
}

type Supervisor interface {
	Init(self erl.PID, args any) InitResult
}

// Impelements [genserver.GenServer] and accepts the callback module for Supervisor
type SupervisorS struct {
	callback Supervisor
}

func (s SupervisorS) Init(self erl.PID, args any) (genserver.InitResult[supervisorState], error) {
	var err error
	erl.ProcessFlag(self, erl.TrapExit, true)
	initResult := s.callback.Init(self, args)
	if initResult.Ignore {
		return genserver.InitResult[supervisorState]{}, exitreason.Ignore
	}
	// checks for duplicate childIDs, which is an error
	children, err := newChildSpecs(initResult.ChildSpecs)
	if err != nil {
		return genserver.InitResult[supervisorState]{}, exitreason.Shutdown(err)
	}
	state := supervisorState{
		children:   children,
		childSpecs: initResult.ChildSpecs,
		flags:      initResult.SupFlags,
	}

	err = s.startChildren(self, state.children)
	if err != nil {
		erl.DebugPrintf("Supervisor[%v] error starting children: %v", self, err)
		if exitreason.IsShutdown(err) {
			return genserver.InitResult[supervisorState]{}, err
		} else {
			return genserver.InitResult[supervisorState]{}, exitreason.Shutdown(err)
		}
	}

	erl.DebugPrintf("Supervisor[%v] done initializing: %+v", self, state.children)
	return genserver.InitResult[supervisorState]{State: state}, nil
}

func (s SupervisorS) HandleCall(self erl.PID, request any, from genserver.From, state supervisorState) (genserver.CallResult[supervisorState], error) {
	// TODO: IMPLEMENT
	return genserver.CallResult[supervisorState]{Msg: "not implemented", State: state}, nil
}

func (s SupervisorS) HandleInfo(self erl.PID, request any, state supervisorState) (genserver.InfoResult[supervisorState], error) {
	switch msg := request.(type) {
	case erl.ExitMsg:
		erl.Logger.Printf("GenServer %v got exit msg: %+v", self, msg)
		return s.restartChild(self, msg, state)
	default:
		erl.Logger.Printf("%v got unknown msg: %+v", self, msg)
	}

	return genserver.InfoResult[supervisorState]{State: state}, nil
}

func (s SupervisorS) restartChild(self erl.PID, msg erl.ExitMsg, state supervisorState) (genserver.InfoResult[supervisorState], error) {
	childSpec, err := state.children.findByPID(msg.Proc) // findChildByPID(msg.Proc)
	// NOTE: For now, if we don't find the child, we can assume this is an exit from a process we already restarted?
	if err != nil {
		erl.DebugPrintf("Supervisor[%v]: no matching pid found", self, err)
		return genserver.InfoResult[supervisorState]{State: state}, nil
	}

	if childSpec.Restart == Temporary {
		// temporary processes are never restarted, and do not contribute to restart intensity calculations
		state.children.delete(childSpec.ID)
		return genserver.InfoResult[supervisorState]{State: state}, nil
	} else if childSpec.Restart == Permanent {
		newState, err := s.processChildRestart(self, childSpec, state)
		return genserver.InfoResult[supervisorState]{State: newState}, err
	} else if exitreason.IsShutdown(msg.Reason) || errors.Is(msg.Reason, exitreason.Normal) || errors.Is(msg.Reason, exitreason.SupervisorShutdown) {
		// if we're a transient restart child, and the exitreason is any of the above, we don't restart
		erl.DebugPrintf("Supervisor[%v] child is transient and had a clean exit, don't restart: %+v", self, msg)
		state.children.delete(childSpec.ID)
		return genserver.InfoResult[supervisorState]{State: state}, nil
	} else {
		erl.DebugPrintf("Supervisor[%v] is restrating child: %+v", self, msg)
		newState, err := s.processChildRestart(self, childSpec, state)
		return genserver.InfoResult[supervisorState]{State: newState}, err
	}
}

func (s SupervisorS) processChildRestart(self erl.PID, childSpec ChildSpec, state supervisorState) (supervisorState, error) {
	erl.DebugPrintf("Supervisor[%v] restarting child: %+v", self, childSpec.ID)
	var err error
	state, err = state.addRestart()
	if err != nil {
		return state, err
	}

	switch state.flags.Strategy {
	case OneForOne:
		c, err := s.startChild(self, childSpec)
		if err != nil {
			return state, err
		}
		state.children.update(c)
		// return state, nil
	case OneForAll:
		childSpecs, _ := s.stopChildren(self, state.children.reverse())
		startOrdered := childSpecs.reverse()
		s.startChildren(self, startOrdered)
		state.children = startOrdered

	case RestForOne:
		keep, restart, err := state.children.split(childSpec.ID)
		if err != nil {
			return state, err
		}
		restarted, _ := s.stopChildren(self, restart.reverse())
		startOrdered := restarted.reverse()
		s.startChildren(self, startOrdered)

		// there shouldn't be any dupes so ignoring the error here
		keep.append(startOrdered)
		state.children = keep

	default:
		return state, fmt.Errorf("should not have reached default case processChildRestart")
	}
	return state, nil
}

func (s SupervisorS) HandleCast(self erl.PID, arg any, state supervisorState) (genserver.CastResult[supervisorState], error) {
	// TODO: IMPLEMENT
	return genserver.CastResult[supervisorState]{State: state}, nil
}

func (s SupervisorS) HandleContinue(self erl.PID, continuation any, state supervisorState) (supervisorState, any, error) {
	// TODO: IMPLEMENT
	return state, nil, nil
}

func (s SupervisorS) Terminate(self erl.PID, arg error, state supervisorState) {
	erl.Logger.Printf("stopping supervisor: %v", self)

	s.stopChildren(self, state.children.reverse())
}

func (s SupervisorS) startChild(self erl.PID, child ChildSpec) (cs ChildSpec, err error) {
	defer func() {
		if r := recover(); r != nil {
			e, ok := r.(error)
			if !ok {
				err = exitreason.Exception(fmt.Errorf("panic starting child: %v", r))
			} else {
				if !exitreason.IsException(e) {
					err = exitreason.Exception(e)
				} else {
					err = e
				}
			}

		}
	}()
	childPID, err := child.Start(self)

	switch {
	case err == nil:
		child.pid = childPID

		return child, nil
	case errors.Is(err, exitreason.Ignore):
		erl.DebugPrintf("child returend :ignore exitreason")
		child.pid = erl.UndefinedPID
		child.ignored = true
		return child, nil
	default:
		return child, exitreason.Exception(err)
	}
}

func (s SupervisorS) stopChildren(self erl.PID, children *childSpecs) (*childSpecs, error) {
	for _, child := range children.list() {

		c, ok := s.terminateChild(self, child)
		if ok {
			children.update(c)
		} else {
			children.delete(child.ID)
		}
	}
	return children, nil
}

func (s SupervisorS) terminateChild(self erl.PID, c ChildSpec) (ChildSpec, bool) {
	listen := make(chan childKillerDoneMsg, 1)
	if erl.IsAlive(c.pid) {
		erl.DebugPrintf("Supervisor[%v]: stopping child %v", self, c.ID)
		erl.SpawnLink(self, &childKiller{parent: listen, parentPID: self, child: c})
	} else {
		erl.DebugPrintf("Supervisor[%v] child %v is not started, mark as terminated", self, c.ID)
		listen <- childKillerDoneMsg{err: nil}
	}

	killResult := <-listen
	if killResult.err != nil {
		erl.Logger.Printf("Supervisor[%v] child %s exited with error: %v ", self, c.ID, killResult.err)
	}
	c.terminated = true
	if c.Restart == Temporary {
		return c, false
	} else {
		return c, true
	}
}

func (s SupervisorS) startChildren(self erl.PID, children *childSpecs) error {
	for _, childSpec := range children.list() {
		child, err := s.startChild(self, childSpec)
		if err != nil {
			// err wasn't nil or ignore, so rollup everything we've already started and return the error
			// we encountered
			erl.DebugPrintf("Supervisor[%v]: child returned an error: %v", self, err)
			// ignoring return here since we're stopping
			s.stopChildren(self, children.reverse())
			return err

		}

		children.update(child)

	}
	return nil
}
