package dynsup

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/uberbrodt/erl-go/chronos"
	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
	"github.com/uberbrodt/erl-go/erl/genserver"
	"github.com/uberbrodt/erl-go/erl/supervisor"
)

var errRestartsExceeded = errors.New("dynamic supervisor restart intensity exceeded")

// Interface compliance check.
var _ genserver.GenServer[dynSupState] = dynSupServer{}

// dynSupServer is the GenServer implementation for the dynamic supervisor.
type dynSupServer struct {
	callback DynamicSupervisor
}

// dynSupState holds the dynamic supervisor's internal state.
type dynSupState struct {
	children map[erl.PID]supervisor.ChildSpec
	flags    SupFlags
	restarts []time.Time
}

// Init implements [genserver.GenServer.Init].
//
// Sets TrapExit, calls the DynamicSupervisor callback's Init, handles Ignore,
// and initializes an empty children map.
func (s dynSupServer) Init(self erl.PID, args any) (genserver.InitResult[dynSupState], error) {
	erl.ProcessFlag(self, erl.TrapExit, true)

	initResult := s.callback.Init(self, args)
	if initResult.Ignore {
		return genserver.InitResult[dynSupState]{}, exitreason.Ignore
	}

	state := dynSupState{
		children: make(map[erl.PID]supervisor.ChildSpec),
		flags:    initResult.SupFlags,
	}

	erl.DebugPrintf("DynSup[%v] initialized with flags: %+v", self, state.flags)
	return genserver.InitResult[dynSupState]{State: state}, nil
}

// HandleCall implements [genserver.GenServer.HandleCall].
func (s dynSupServer) HandleCall(self erl.PID, request any, from genserver.From, state dynSupState) (genserver.CallResult[dynSupState], error) {
	switch req := request.(type) {
	case startChildRequest:
		pid, newState, err := s.doStartChild(self, req.spec, state)
		return genserver.CallResult[dynSupState]{
			Msg:   startChildResponse{pid: pid, err: err},
			State: newState,
		}, nil

	case terminateChildRequest:
		newState, err := s.doTerminateChild(self, req.pid, state)
		return genserver.CallResult[dynSupState]{
			Msg:   terminateChildResponse{err: err},
			State: newState,
		}, nil

	case whichChildrenRequest:
		children := s.doWhichChildren(state)
		return genserver.CallResult[dynSupState]{
			Msg:   whichChildrenResponse{children: children},
			State: state,
		}, nil

	case countChildrenRequest:
		count := s.doCountChildren(state)
		return genserver.CallResult[dynSupState]{
			Msg:   countChildrenResponse{count: count},
			State: state,
		}, nil

	default:
		return genserver.CallResult[dynSupState]{
			Msg:   "unknown request",
			State: state,
		}, nil
	}
}

// HandleCast implements [genserver.GenServer.HandleCast]. No-op.
func (s dynSupServer) HandleCast(self erl.PID, request any, state dynSupState) (genserver.CastResult[dynSupState], error) {
	return genserver.CastResult[dynSupState]{State: state}, nil
}

// HandleInfo implements [genserver.GenServer.HandleInfo].
//
// Handles [erl.ExitMsg] when children terminate, deciding whether to restart
// based on the child's restart type and exit reason.
func (s dynSupServer) HandleInfo(self erl.PID, msg any, state dynSupState) (genserver.InfoResult[dynSupState], error) {
	switch m := msg.(type) {
	case erl.ExitMsg:
		return s.handleChildExit(self, m, state)
	default:
		erl.DebugPrintf("DynSup[%v] got unknown msg: %+v", self, msg)
	}
	return genserver.InfoResult[dynSupState]{State: state}, nil
}

// HandleContinue implements [genserver.GenServer.HandleContinue]. No-op.
func (s dynSupServer) HandleContinue(self erl.PID, continuation any, state dynSupState) (dynSupState, any, error) {
	return state, nil, nil
}

// Terminate implements [genserver.GenServer.Terminate].
//
// Concurrently shuts down all children by spawning a childKiller for each,
// then waiting on a shared channel for all to complete.
func (s dynSupServer) Terminate(self erl.PID, reason error, state dynSupState) {
	erl.Logger.Printf("DynSup[%v] terminating: %v", self, reason)

	if len(state.children) == 0 {
		return
	}

	done := make(chan childKillerDoneMsg, len(state.children))

	for pid, spec := range state.children {
		if erl.IsAlive(pid) {
			erl.Spawn(&childKiller{
				parent:    done,
				parentPID: self,
				childPID:  pid,
				shutdown:  spec.Shutdown,
				restart:   spec.Restart,
			})
		} else {
			done <- childKillerDoneMsg{}
		}
	}

	// Wait for all children to terminate.
	for range len(state.children) {
		result := <-done
		if result.err != nil {
			erl.Logger.Printf("DynSup[%v] child exited with error during shutdown: %v", self, result.err)
		}
	}
}

// =============================================================================
// Internal handlers
// =============================================================================

// doStartChild starts a new child process.
func (s dynSupServer) doStartChild(self erl.PID, spec supervisor.ChildSpec, state dynSupState) (pid erl.PID, newState dynSupState, retErr error) {
	// MaxChildren check
	if state.flags.MaxChildren > 0 && len(state.children) >= state.flags.MaxChildren {
		return erl.PID{}, state, ErrMaxChildren
	}

	// Start child with panic recovery
	defer func() {
		if r := recover(); r != nil {
			e, ok := r.(error)
			if !ok {
				retErr = exitreason.Exception(fmt.Errorf("panic starting child: %v", r))
			} else {
				if !exitreason.IsException(e) {
					retErr = exitreason.Exception(e)
				} else {
					retErr = e
				}
			}
			pid = erl.PID{}
			newState = state
		}
	}()

	childPID, err := spec.Start(self)

	switch {
	case err == nil:
		state.children[childPID] = spec
		return childPID, state, nil
	case errors.Is(err, exitreason.Ignore):
		erl.DebugPrintf("DynSup[%v] child returned :ignore", self)
		return erl.PID{}, state, nil
	default:
		return erl.PID{}, state, err
	}
}

// doTerminateChild terminates a child by PID and removes it from the map.
func (s dynSupServer) doTerminateChild(self erl.PID, pid erl.PID, state dynSupState) (dynSupState, error) {
	spec, ok := state.children[pid]
	if !ok {
		return state, ErrNotFound
	}

	if erl.IsAlive(pid) {
		done := make(chan childKillerDoneMsg, 1)
		erl.Spawn(&childKiller{
			parent:    done,
			parentPID: self,
			childPID:  pid,
			shutdown:  spec.Shutdown,
			restart:   spec.Restart,
		})
		result := <-done
		if result.err != nil {
			erl.Logger.Printf("DynSup[%v] child %v exited with error: %v", self, pid, result.err)
		}
	}

	delete(state.children, pid)
	return state, nil
}

// doWhichChildren returns information about all children.
func (s dynSupServer) doWhichChildren(state dynSupState) []supervisor.ChildInfo {
	result := make([]supervisor.ChildInfo, 0, len(state.children))
	for pid, spec := range state.children {
		status := supervisor.ChildRunning
		activePID := pid
		if !erl.IsAlive(pid) {
			status = supervisor.ChildUndefined
			activePID = erl.PID{}
		}
		result = append(result, supervisor.ChildInfo{
			PID:     activePID,
			Type:    spec.Type,
			Status:  status,
			Restart: spec.Restart,
		})
	}
	return result
}

// doCountChildren returns counts of children by category.
func (s dynSupServer) doCountChildren(state dynSupState) supervisor.ChildCount {
	count := supervisor.ChildCount{
		Specs: len(state.children),
	}
	for pid, spec := range state.children {
		if spec.Type == supervisor.SupervisorChild {
			count.Supervisors++
		} else {
			count.Workers++
		}
		if erl.IsAlive(pid) {
			count.Active++
		}
	}
	return count
}

// handleChildExit processes an ExitMsg from a child process.
func (s dynSupServer) handleChildExit(self erl.PID, msg erl.ExitMsg, state dynSupState) (genserver.InfoResult[dynSupState], error) {
	spec, ok := state.children[msg.Proc]
	if !ok {
		// Unrecognized PID — silently ignore (could be stale ExitMsg from terminated child)
		erl.DebugPrintf("DynSup[%v] ignoring ExitMsg from unmanaged PID: %v", self, msg.Proc)
		return genserver.InfoResult[dynSupState]{State: state}, nil
	}

	// Remove the old PID entry
	delete(state.children, msg.Proc)

	switch {
	case spec.Restart == supervisor.Temporary:
		// Temporary: never restart, don't count toward intensity
		return genserver.InfoResult[dynSupState]{State: state}, nil

	case spec.Restart == supervisor.Permanent:
		// Permanent: always restart
		return s.restartChild(self, spec, state)

	case exitreason.IsShutdown(msg.Reason) ||
		errors.Is(msg.Reason, exitreason.Normal) ||
		errors.Is(msg.Reason, exitreason.SupervisorShutdown):
		// Transient with clean exit: don't restart
		return genserver.InfoResult[dynSupState]{State: state}, nil

	default:
		// Transient with abnormal exit: restart
		return s.restartChild(self, spec, state)
	}
}

// restartChild attempts to restart a child and checks intensity.
func (s dynSupServer) restartChild(self erl.PID, spec supervisor.ChildSpec, state dynSupState) (genserver.InfoResult[dynSupState], error) {
	var err error
	state, err = addRestart(state)
	if err != nil {
		return genserver.InfoResult[dynSupState]{State: state}, err
	}

	_, newState, startErr := s.doStartChild(self, spec, state)
	if startErr != nil {
		// Restart failed — propagate error to terminate supervisor
		return genserver.InfoResult[dynSupState]{State: newState},
			fmt.Errorf("DynSup restart failed for child: %w", startErr)
	}

	return genserver.InfoResult[dynSupState]{State: newState}, nil
}

// addRestart records a restart and checks if intensity is exceeded.
func addRestart(state dynSupState) (dynSupState, error) {
	now := chronos.Now("")
	state.restarts = append(state.restarts, now)
	period := now.Add(-chronos.Dur(fmt.Sprintf("%ds", state.flags.Period)))

	c := 0
	trim := -1
	for idx, r := range state.restarts {
		if r.After(period) {
			c++
			if c > state.flags.Intensity {
				erl.DebugPrintf("dynamic supervisor restart intensity exceeded")
				return state, errRestartsExceeded
			}
		} else {
			trim = idx
		}
	}
	if trim >= 0 {
		state.restarts = slices.Delete(state.restarts, 0, trim+1)
	}
	return state, nil
}
