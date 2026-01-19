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

// SupFlagsS configures supervisor behavior including restart strategy and intensity limits.
// Create using [NewSupFlags] with functional options.
type SupFlagsS struct {
	// Strategy determines which children to restart when one fails.
	// Default: OneForOne
	Strategy Strategy

	// Period is the time window in seconds for counting restarts.
	// Restarts older than Period seconds are not counted toward intensity.
	// Default: 5
	Period int

	// Intensity is the maximum number of restarts allowed within Period seconds.
	// If this limit is exceeded, the supervisor terminates itself and all children.
	// Set to 0 to terminate on the first child failure.
	// Default: 1
	Intensity int
}

// SupFlag is a functional option for configuring [SupFlagsS].
type SupFlag func(flags SupFlagsS) SupFlagsS

// SetStrategy sets the supervisor's restart strategy.
// See [OneForOne], [OneForAll], and [RestForOne] for available strategies.
func SetStrategy(strategy Strategy) SupFlag {
	return func(flags SupFlagsS) SupFlagsS {
		flags.Strategy = strategy
		return flags
	}
}

// SetPeriod sets the restart evaluation window in seconds.
// The supervisor tracks restarts within this rolling window.
// Restarts older than Period seconds are forgotten and don't count toward intensity.
func SetPeriod(period int) SupFlag {
	return func(flags SupFlagsS) SupFlagsS {
		flags.Period = period
		return flags
	}
}

// SetIntensity sets the maximum number of restarts allowed within the period.
// If more than Intensity restarts occur within Period seconds, the supervisor
// terminates itself (and all children), propagating the failure up the supervision tree.
//
// This prevents infinite restart loops when a child has a persistent problem.
// Setting intensity to 0 means the supervisor terminates on the first child failure.
func SetIntensity(intensity int) SupFlag {
	return func(flags SupFlagsS) SupFlagsS {
		flags.Intensity = intensity
		return flags
	}
}

// NewSupFlags creates supervisor flags with the given options.
// Default values: Strategy=OneForOne, Period=5, Intensity=1.
//
// Example:
//
//	flags := supervisor.NewSupFlags(
//		supervisor.SetStrategy(supervisor.OneForAll),
//		supervisor.SetIntensity(3),
//		supervisor.SetPeriod(10),
//	)
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

// InitResult is returned by the [Supervisor.Init] callback to configure the supervisor.
type InitResult struct {
	// SupFlags configures the supervisor's restart strategy and intensity limits.
	SupFlags SupFlagsS

	// ChildSpecs defines the children to start, in order. Children are started
	// sequentially; if any child fails to start, previously started children
	// are stopped and the supervisor fails to start.
	ChildSpecs []ChildSpec

	// Ignore, if true, causes the supervisor to exit with [exitreason.Ignore],
	// preventing it from starting. The calling process receives an error but
	// no exit signal. Useful for conditional supervision based on configuration.
	Ignore bool
}

// Supervisor is the callback interface for dynamic supervisor configuration.
// Implement this interface when children need to be determined at runtime
// rather than at compile time.
//
// For static child lists, use [StartDefaultLink] instead which doesn't require
// implementing this interface.
type Supervisor interface {
	// Init is called when the supervisor starts to obtain configuration.
	// Return the supervisor flags and child specifications.
	//
	// Set Ignore=true in the result to cancel supervisor startup without error
	// propagation (the supervisor exits with exitreason.Ignore).
	//
	// The self parameter is the supervisor's own PID, which can be used for
	// registration or other setup, but children should NOT be started here
	// directly; return them in ChildSpecs instead.
	Init(self erl.PID, args any) InitResult
}

// SupervisorS implements [genserver.GenServer] and manages child processes according
// to the configured strategy and restart rules. Users typically don't interact with
// this type directly; use [StartDefaultLink] or [StartLink] instead.
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

