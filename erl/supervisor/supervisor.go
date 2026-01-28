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

// Internal request types for HandleCall - used by public API functions
type (
	whichChildrenRequest  struct{}
	countChildrenRequest  struct{}
	startChildRequest     struct{ spec ChildSpec }
	terminateChildRequest struct{ childID string }
	restartChildRequest   struct{ childID string }
	deleteChildRequest    struct{ childID string }
)

// Internal response types for HandleCall - returned to public API functions
type (
	whichChildrenResponse struct{ children []ChildInfo }
	countChildrenResponse struct{ count ChildCount }
	startChildResponse    struct {
		pid erl.PID
		err error
	}
	terminateChildResponse struct{ err error }
	restartChildResponse   struct {
		pid erl.PID
		err error
	}
	deleteChildResponse struct{ err error }
)

// SupFlagsS configures supervisor behavior including restart strategy and intensity limits.
//
// Create using [NewSupFlags] with functional options:
//
//	flags := supervisor.NewSupFlags(
//		supervisor.SetStrategy(supervisor.OneForAll),
//		supervisor.SetIntensity(3),
//		supervisor.SetPeriod(10),
//	)
//
// The default values (Strategy=OneForOne, Period=5, Intensity=1) provide
// conservative restart protection suitable for most use cases.
type SupFlagsS struct {
	// Strategy determines which children to restart when one fails.
	//
	// Available strategies:
	//   - [OneForOne]: Only restart the failed child (default)
	//   - [OneForAll]: Restart all children when any fails
	//   - [RestForOne]: Restart failed child and all children started after it
	Strategy Strategy

	// Period is the time window in seconds for counting restarts.
	// Restarts older than Period seconds are not counted toward intensity.
	//
	// Works with Intensity to prevent infinite restart loops: if more than
	// Intensity restarts occur within Period seconds, the supervisor terminates.
	//
	// Default: 5 seconds
	Period int

	// Intensity is the maximum number of restarts allowed within Period seconds.
	// If this limit is exceeded, the supervisor terminates itself and all children,
	// propagating the failure up the supervision tree.
	//
	// This prevents infinite restart loops when a child has a persistent problem
	// that causes it to crash repeatedly.
	//
	// Set to 0 to terminate on the first child restart (very strict).
	// Set higher for children that may have transient startup issues.
	//
	// Default: 1
	Intensity int
}

// SupFlag is a functional option for configuring [SupFlagsS].
// Use with [NewSupFlags].
type SupFlag func(flags SupFlagsS) SupFlagsS

// SetStrategy sets the supervisor's restart strategy.
//
// Available strategies:
//   - [OneForOne]: Only restart the failed child (default)
//   - [OneForAll]: Restart all children when any fails
//   - [RestForOne]: Restart failed child and all children started after it
//
// Example:
//
//	flags := supervisor.NewSupFlags(
//		supervisor.SetStrategy(supervisor.RestForOne),
//	)
func SetStrategy(strategy Strategy) SupFlag {
	return func(flags SupFlagsS) SupFlagsS {
		flags.Strategy = strategy
		return flags
	}
}

// SetPeriod sets the restart evaluation window in seconds.
//
// The supervisor tracks restarts within this rolling window. Restarts older
// than Period seconds are forgotten and don't count toward intensity.
//
// Example: With Period=60 and Intensity=5, up to 5 restarts are allowed
// within any 60-second window.
//
// Default: 5 seconds
func SetPeriod(period int) SupFlag {
	return func(flags SupFlagsS) SupFlagsS {
		flags.Period = period
		return flags
	}
}

// SetIntensity sets the maximum number of restarts allowed within the period.
//
// If more than Intensity restarts occur within Period seconds, the supervisor
// terminates itself (and all children), propagating the failure up the supervision tree.
//
// This prevents infinite restart loops when a child has a persistent problem.
//
// Setting intensity to 0 means the supervisor terminates on the first child restart.
// This is very strict and typically only used when any child failure indicates
// a fundamental problem.
//
// Default: 1
//
// Example:
//
//	// Allow up to 10 restarts within 60 seconds
//	flags := supervisor.NewSupFlags(
//		supervisor.SetIntensity(10),
//		supervisor.SetPeriod(60),
//	)
func SetIntensity(intensity int) SupFlag {
	return func(flags SupFlagsS) SupFlagsS {
		flags.Intensity = intensity
		return flags
	}
}

// NewSupFlags creates supervisor flags with the given options.
//
// Default values:
//   - Strategy: [OneForOne]
//   - Period: 5 seconds
//   - Intensity: 1 restart
//
// These defaults provide conservative restart protection: only the failed
// child restarts, and if it fails more than once within 5 seconds, the
// supervisor terminates.
//
// Examples:
//
//	// Use defaults (OneForOne, 1 restart per 5 seconds)
//	flags := supervisor.NewSupFlags()
//
//	// Custom configuration
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
//
// The ChildSpecs are started in order. If any child fails to start (returns an error
// other than [exitreason.Ignore]), previously started children are stopped and the
// supervisor fails to start.
type InitResult struct {
	// SupFlags configures the supervisor's restart strategy and intensity limits.
	// Use [NewSupFlags] to create with defaults and customize as needed.
	SupFlags SupFlagsS

	// ChildSpecs defines the children to start, in order.
	//
	// Children are started sequentially in slice order. The order matters for:
	//   - [RestForOne] strategy (later children depend on earlier ones)
	//   - Shutdown order (children are stopped in reverse start order)
	//
	// If any child fails to start, previously started children are stopped
	// (rollback) and the supervisor fails to start.
	ChildSpecs []ChildSpec

	// Ignore, if true, causes the supervisor to exit with [exitreason.Ignore],
	// preventing it from starting. The calling process receives an error but
	// no exit signal is propagated.
	//
	// Use for conditional supervision based on configuration:
	//
	//	func (s MySup) Init(self erl.PID, args any) supervisor.InitResult {
	//		if !config.FeatureEnabled {
	//			return supervisor.InitResult{Ignore: true}
	//		}
	//		// ... normal initialization
	//	}
	Ignore bool
}

// Supervisor is the callback interface for dynamic supervisor configuration.
//
// Implement this interface when children need to be determined at runtime
// rather than at compile time. For static child lists known at compile time,
// use [StartDefaultLink] instead which doesn't require implementing this interface.
//
// Example:
//
//	type MySupervisor struct{}
//
//	func (s MySupervisor) Init(self erl.PID, args any) supervisor.InitResult {
//		config := args.(MyConfig)
//		children := make([]supervisor.ChildSpec, config.WorkerCount)
//		for i := range children {
//			id := fmt.Sprintf("worker_%d", i)
//			children[i] = supervisor.NewChildSpec(id, workerStartFn)
//		}
//		return supervisor.InitResult{
//			SupFlags:   supervisor.NewSupFlags(),
//			ChildSpecs: children,
//		}
//	}
//
//	// Usage:
//	supPID, err := supervisor.StartLink(self, MySupervisor{}, myConfig)
type Supervisor interface {
	// Init is called when the supervisor starts to obtain configuration.
	//
	// Parameters:
	//   - self: The supervisor's own PID (can be used for registration or logging)
	//   - args: Arguments passed to [StartLink]
	//
	// Return the supervisor flags and child specifications in [InitResult].
	// Set Ignore=true to cancel supervisor startup without error propagation.
	//
	// Important: Do NOT start children directly in Init. Return them in ChildSpecs
	// and let the supervisor start them. This ensures proper linking and monitoring.
	Init(self erl.PID, args any) InitResult
}

// SupervisorS implements [genserver.GenServer] and manages child processes according
// to the configured strategy and restart rules.
//
// Users typically don't interact with this type directly. Use [StartDefaultLink]
// or [StartLink] to create and start supervisors.
//
// The supervisor:
//   - Sets TrapExit to receive exit signals as [erl.ExitMsg] messages
//   - Starts all children in order during Init
//   - Monitors children via links and handles their exits
//   - Restarts children according to strategy and restart type
//   - Tracks restart frequency and terminates if intensity exceeded
//   - Stops all children in reverse order during Terminate
type SupervisorS struct {
	callback Supervisor
}

// Init implements [genserver.GenServer.Init].
//
// Sets up the supervisor by:
//  1. Enabling TrapExit to receive child exit signals as messages
//  2. Calling the callback's Init to get configuration
//  3. Validating child specs (no duplicate IDs)
//  4. Starting all children in order
//
// If any child fails to start, previously started children are stopped
// and an error is returned, causing the supervisor to fail.
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

// HandleCall implements [genserver.GenServer.HandleCall].
//
// Handles dynamic child management requests:
//   - [WhichChildren]: List children and their status
//   - [CountChildren]: Count children by category
//   - [StartChild]: Add and start a new child
//   - [TerminateChild]: Stop a running child
//   - [RestartChild]: Restart a terminated child
//   - [DeleteChild]: Remove a child specification
func (s SupervisorS) HandleCall(self erl.PID, request any, from genserver.From, state supervisorState) (genserver.CallResult[supervisorState], error) {
	switch req := request.(type) {
	case whichChildrenRequest:
		children := s.doWhichChildren(state)
		return genserver.CallResult[supervisorState]{
			Msg:   whichChildrenResponse{children: children},
			State: state,
		}, nil

	case countChildrenRequest:
		count := s.doCountChildren(state)
		return genserver.CallResult[supervisorState]{
			Msg:   countChildrenResponse{count: count},
			State: state,
		}, nil

	case startChildRequest:
		pid, newState, err := s.doStartChild(self, req.spec, state)
		return genserver.CallResult[supervisorState]{
			Msg:   startChildResponse{pid: pid, err: err},
			State: newState,
		}, nil

	case terminateChildRequest:
		newState, err := s.doTerminateChild(self, req.childID, state)
		return genserver.CallResult[supervisorState]{
			Msg:   terminateChildResponse{err: err},
			State: newState,
		}, nil

	case restartChildRequest:
		pid, newState, err := s.doRestartChild(self, req.childID, state)
		return genserver.CallResult[supervisorState]{
			Msg:   restartChildResponse{pid: pid, err: err},
			State: newState,
		}, nil

	case deleteChildRequest:
		newState, err := s.doDeleteChild(req.childID, state)
		return genserver.CallResult[supervisorState]{
			Msg:   deleteChildResponse{err: err},
			State: newState,
		}, nil

	default:
		return genserver.CallResult[supervisorState]{
			Msg:   "unknown request",
			State: state,
		}, nil
	}
}

// HandleInfo implements [genserver.GenServer.HandleInfo].
//
// Handles [erl.ExitMsg] when children terminate. Delegates to restartChild
// to decide whether and how to restart based on:
//   - Child's restart type (Permanent, Transient, Temporary)
//   - Exit reason (Normal, Shutdown, Exception, etc.)
//   - Supervisor's restart strategy
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

// restartChild handles a child exit and decides whether to restart.
//
// Decision logic:
//   - Temporary: Never restart, remove from children
//   - Permanent: Always restart
//   - Transient: Restart only on abnormal exit (not Normal/Shutdown/SupervisorShutdown)
//
// Returns error if restart intensity is exceeded, causing supervisor to terminate.
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

// processChildRestart executes the restart according to strategy.
//
// First increments restart counter and checks intensity. If exceeded, returns
// error to terminate supervisor.
//
// Then executes strategy:
//   - OneForOne: Restart only the failed child
//   - OneForAll: Stop all children (reverse order), restart all (start order)
//   - RestForOne: Stop failed child and later children, restart them in order
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

// HandleCast implements [genserver.GenServer.HandleCast].
//
// Currently not implemented. Future versions may support asynchronous
// supervisor operations.
func (s SupervisorS) HandleCast(self erl.PID, arg any, state supervisorState) (genserver.CastResult[supervisorState], error) {
	// TODO: IMPLEMENT
	return genserver.CastResult[supervisorState]{State: state}, nil
}

// HandleContinue implements [genserver.GenServer.HandleContinue].
//
// Currently not implemented.
func (s SupervisorS) HandleContinue(self erl.PID, continuation any, state supervisorState) (supervisorState, any, error) {
	// TODO: IMPLEMENT
	return state, nil, nil
}

// Terminate implements [genserver.GenServer.Terminate].
//
// Stops all children in reverse start order. Each child is terminated
// according to its [ShutdownOpt] configuration:
//   - Timeout: Wait up to N ms, then kill
//   - BrutalKill: Kill immediately
//   - Infinity: Wait forever
func (s SupervisorS) Terminate(self erl.PID, arg error, state supervisorState) {
	erl.Logger.Printf("stopping supervisor: %v", self)

	s.stopChildren(self, state.children.reverse())
}

// startChild starts a single child process.
//
// Calls the child's Start function and handles the result:
//   - Success: Records PID in child spec
//   - Ignore: Marks child as ignored (not running but tracked)
//   - Error: Wraps in exitreason.Exception and returns
//
// Panics in Start are recovered and converted to exitreason.Exception.
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

// stopChildren stops all children in the given order.
//
// Each child is terminated via terminateChild, which spawns a childKiller
// process to handle the shutdown according to the child's ShutdownOpt.
//
// Temporary children are removed from the returned list; other children
// are kept (marked as terminated) for potential restart.
func (s SupervisorS) stopChildren(self erl.PID, children *childSpecs) (*childSpecs, error) {
	// Take a snapshot of children to iterate over, since we may modify
	// the children list during iteration (deleting Temporary children).
	childList := children.list()
	toDelete := make([]string, 0)

	for _, child := range childList {
		c, ok := s.terminateChild(self, child)
		if ok {
			children.update(c)
		} else {
			// Mark for deletion after iteration completes
			toDelete = append(toDelete, child.ID)
		}
	}

	// Delete temporary children after iteration
	for _, id := range toDelete {
		children.delete(id)
	}

	return children, nil
}

// terminateChild stops a single child process.
//
// Spawns a childKiller process to handle shutdown asynchronously. The
// childKiller:
//  1. Unlinks the child from the supervisor (to avoid exit signal)
//  2. Monitors the child
//  3. Sends exit signal based on ShutdownOpt
//  4. Waits for DownMsg confirming termination
//
// Returns (spec, true) to keep the child for restart, or (spec, false)
// if the child should be removed (Temporary restart type).
func (s SupervisorS) terminateChild(self erl.PID, c ChildSpec) (ChildSpec, bool) {
	listen := make(chan childKillerDoneMsg, 1)
	if erl.IsAlive(c.pid) {
		erl.DebugPrintf("Supervisor[%v]: stopping child %v", self, c.ID)
		// Use Spawn instead of SpawnLink to avoid sending ExitMsg to supervisor
		// when childKiller exits. The supervisor already receives completion
		// notification via the parent channel.
		erl.Spawn(&childKiller{parent: listen, parentPID: self, child: c})
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

// startChildren starts all children in the given order.
//
// If any child fails to start (returns error other than Ignore), stops all
// previously started children and returns the error. This provides rollback
// behavior during supervisor initialization.
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

// =============================================================================
// Dynamic Child Management Handlers
// =============================================================================

// doWhichChildren returns information about all children.
// Called by HandleCall for whichChildrenRequest.
func (s SupervisorS) doWhichChildren(state supervisorState) []ChildInfo {
	specs := state.children.list()
	result := make([]ChildInfo, 0, len(specs))

	for _, spec := range specs {
		info := ChildInfo{
			ID:      spec.ID,
			PID:     spec.pid,
			Type:    spec.Type,
			Restart: spec.Restart,
		}

		// Determine status based on internal state
		switch {
		case spec.ignored:
			info.Status = ChildUndefined
			info.PID = erl.PID{} // Clear PID for undefined children
		case spec.terminated:
			info.Status = ChildTerminated
			info.PID = erl.PID{} // Clear PID for terminated children
		case erl.IsAlive(spec.pid):
			info.Status = ChildRunning
		default:
			// Child was running but has exited (race or not yet processed)
			info.Status = ChildUndefined
			info.PID = erl.PID{}
		}

		result = append(result, info)
	}

	return result
}

// doCountChildren returns counts of children by category.
// Called by HandleCall for countChildrenRequest.
func (s SupervisorS) doCountChildren(state supervisorState) ChildCount {
	specs := state.children.list()
	count := ChildCount{
		Specs: len(specs),
	}

	for _, spec := range specs {
		// Count by type
		if spec.Type == SupervisorChild {
			count.Supervisors++
		} else {
			count.Workers++
		}

		// Count active (running) children
		if !spec.ignored && !spec.terminated && erl.IsAlive(spec.pid) {
			count.Active++
		}
	}

	return count
}

// doStartChild adds and starts a new child.
// Called by HandleCall for startChildRequest.
//
// Returns:
//   - (pid, state, nil): Child started successfully
//   - (_, state, AlreadyStartedError): Child with ID exists and is running
//   - (_, state, ErrAlreadyPresent): Child with ID exists but is not running
//   - (_, state, error): Child failed to start
//
// Note: This operation does NOT affect restart intensity calculations.
func (s SupervisorS) doStartChild(self erl.PID, spec ChildSpec, state supervisorState) (erl.PID, supervisorState, error) {
	// Check if child already exists
	_, existing, err := state.children.get(spec.ID)
	if err == nil {
		// Child exists - check if running
		if !existing.terminated && !existing.ignored && erl.IsAlive(existing.pid) {
			return erl.PID{}, state, AlreadyStartedError{PID: existing.pid}
		}
		return erl.PID{}, state, ErrAlreadyPresent
	}

	// Start the new child
	startedSpec, err := s.startChild(self, spec)
	if err != nil {
		return erl.PID{}, state, err
	}

	// Add to children list
	newChildren := &childSpecs{specs: []ChildSpec{startedSpec}}
	if appendErr := state.children.append(newChildren); appendErr != nil {
		// This shouldn't happen since we checked for duplicates above
		return erl.PID{}, state, appendErr
	}

	return startedSpec.pid, state, nil
}

// doTerminateChild stops a running child but keeps its specification.
// Called by HandleCall for terminateChildRequest.
//
// The child can be restarted later via RestartChild or removed via DeleteChild.
// This operation does NOT affect restart intensity calculations.
//
// Returns:
//   - (state, nil): Child was terminated (or was already not running)
//   - (state, ErrNotFound): No child with the given ID exists
func (s SupervisorS) doTerminateChild(self erl.PID, childID string, state supervisorState) (supervisorState, error) {
	idx, spec, err := state.children.get(childID)
	if err != nil {
		return state, ErrNotFound
	}

	// If already terminated or ignored, nothing to do
	if spec.terminated || spec.ignored || !erl.IsAlive(spec.pid) {
		// Mark as terminated if not already
		spec.terminated = true
		spec.pid = erl.PID{}
		state.children.specs[idx] = spec
		return state, nil
	}

	// Terminate the child using existing terminateChild logic
	// Note: terminateChild spawns childKiller which handles the shutdown
	terminatedSpec, _ := s.terminateChild(self, spec)
	terminatedSpec.terminated = true
	terminatedSpec.pid = erl.PID{} // Clear PID to prevent auto-restart via findByPID
	state.children.specs[idx] = terminatedSpec

	return state, nil
}

// doRestartChild restarts a terminated child.
// Called by HandleCall for restartChildRequest.
//
// Returns:
//   - (pid, state, nil): Child restarted successfully
//   - (_, state, ErrNotFound): No child with the given ID exists
//   - (_, state, ErrRunning): Child is already running
//   - (_, state, error): Child failed to start
func (s SupervisorS) doRestartChild(self erl.PID, childID string, state supervisorState) (erl.PID, supervisorState, error) {
	idx, spec, err := state.children.get(childID)
	if err != nil {
		return erl.UndefinedPID, state, ErrNotFound
	}

	// Check if already running
	if !spec.terminated && !spec.ignored && erl.IsAlive(spec.pid) {
		return erl.UndefinedPID, state, ErrRunning
	}

	// Clear flags and restart the child
	spec.terminated = false
	spec.ignored = false
	startedSpec, err := s.startChild(self, spec)
	if err != nil {
		// Keep the spec but mark as terminated
		spec.terminated = true
		state.children.specs[idx] = spec
		return erl.UndefinedPID, state, err
	}

	state.children.specs[idx] = startedSpec
	return startedSpec.pid, state, nil
}

// doDeleteChild removes a child specification entirely.
// Called by HandleCall for deleteChildRequest.
//
// The child must not be running. Use TerminateChild first if needed.
//
// Returns:
//   - (state, nil): Child specification was removed
//   - (state, ErrNotFound): No child with the given ID exists
//   - (state, ErrRunning): Child is still running
func (s SupervisorS) doDeleteChild(childID string, state supervisorState) (supervisorState, error) {
	_, spec, err := state.children.get(childID)
	if err != nil {
		return state, ErrNotFound
	}

	// Cannot delete running child
	if !spec.terminated && !spec.ignored && erl.IsAlive(spec.pid) {
		return state, ErrRunning
	}

	// Remove from children
	state.children.delete(childID)
	return state, nil
}




