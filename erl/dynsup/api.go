package dynsup

import (
	"fmt"
	"time"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/genserver"
	"github.com/uberbrodt/erl-go/erl/supervisor"
	"github.com/uberbrodt/erl-go/erl/timeout"
)

type linkOpts struct {
	name erl.Name
}

// LinkOpts is a functional option for configuring dynamic supervisor startup.
// Use with [StartDefaultLink] or [StartLink].
type LinkOpts func(flags linkOpts) linkOpts

// SetName registers the dynamic supervisor under the given name, allowing it to be
// looked up via [erl.WhereIs] or addressed by name instead of PID.
func SetName(name erl.Name) LinkOpts {
	return func(flags linkOpts) linkOpts {
		flags.name = name
		return flags
	}
}

// StartDefaultLink starts a dynamic supervisor with the given flags and zero children.
//
// This is the simplest way to create a dynamic supervisor. For custom initialization
// logic, use [StartLink] with a [DynamicSupervisor] callback.
//
// The supervisor is linked to the calling process (self).
//
// Example:
//
//	flags := dynsup.NewSupFlags(
//		dynsup.SetIntensity(3),
//		dynsup.SetPeriod(5),
//		dynsup.SetMaxChildren(100),
//	)
//
//	supPID, err := dynsup.StartDefaultLink(self, flags)
func StartDefaultLink(self erl.PID, flags SupFlags, optFuns ...LinkOpts) (erl.PID, error) {
	ds := defaultDynSup{flags: flags}
	return StartLink(self, ds, nil, optFuns...)
}

// StartLink starts a dynamic supervisor with a custom callback module.
//
// The callback's [DynamicSupervisor.Init] method is invoked with the provided args
// to obtain the [SupFlags]. The supervisor starts with zero children; add children
// at runtime using [StartChild].
//
// The supervisor is linked to the calling process (self).
//
// Example:
//
//	supPID, err := dynsup.StartLink(self, MyDynSup{}, config,
//		dynsup.SetName("my_dynsup"))
func StartLink(self erl.PID, callback DynamicSupervisor, args any, optFuns ...LinkOpts) (erl.PID, error) {
	opts := linkOpts{}

	for _, fn := range optFuns {
		opts = fn(opts)
	}

	gsOpts := make([]genserver.StartOpt, 0)

	if opts.name != "" {
		gsOpts = append(gsOpts, genserver.SetName(opts.name))
	}

	gsOpts = append(gsOpts, genserver.SetStartTimeout(timeout.Infinity))

	sup := dynSupServer{
		callback: callback,
	}

	return genserver.StartLink[dynSupState](self, sup, args, gsOpts...)
}

// StartChild starts a new child process under the dynamic supervisor.
//
// Returns the child's PID on success. Possible errors:
//   - [ErrMaxChildren]: The supervisor has reached its MaxChildren limit
//   - Other errors: The child failed to start
//
// Example:
//
//	spec := dynsup.NewChildSpec(func(sup erl.PID) (erl.PID, error) {
//		return genserver.StartLink[State](sup, MyWorker{}, config)
//	})
//	pid, err := dynsup.StartChild(supPID, spec)
func StartChild(dest erl.Dest, spec supervisor.ChildSpec) (erl.PID, error) {
	return StartChildTimeout(dest, spec, DefaultAPITimeout)
}

// StartChildTimeout is like [StartChild] but with a custom timeout.
func StartChildTimeout(dest erl.Dest, spec supervisor.ChildSpec, tout time.Duration) (erl.PID, error) {
	self := erl.RootPID()

	result, err := genserver.Call(self, dest, startChildRequest{spec: spec}, tout)
	if err != nil {
		return erl.PID{}, err
	}

	resp, ok := result.(startChildResponse)
	if !ok {
		return erl.PID{}, fmt.Errorf("unexpected response type: %T", result)
	}

	return resp.pid, resp.err
}

// TerminateChild terminates a child process by PID.
//
// The child is removed from the supervisor after termination.
// Unlike the regular supervisor, there is no separate "delete" step.
//
// Returns:
//   - nil: Child was terminated and removed
//   - [ErrNotFound]: The given PID is not a child of this supervisor
//
// Example:
//
//	err := dynsup.TerminateChild(supPID, childPID)
func TerminateChild(dest erl.Dest, pid erl.PID) error {
	return TerminateChildTimeout(dest, pid, DefaultAPITimeout)
}

// TerminateChildTimeout is like [TerminateChild] but with a custom timeout.
func TerminateChildTimeout(dest erl.Dest, pid erl.PID, tout time.Duration) error {
	self := erl.RootPID()

	result, err := genserver.Call(self, dest, terminateChildRequest{pid: pid}, tout)
	if err != nil {
		return err
	}

	resp, ok := result.(terminateChildResponse)
	if !ok {
		return fmt.Errorf("unexpected response type: %T", result)
	}

	return resp.err
}

// WhichChildren returns information about all children managed by the dynamic supervisor.
//
// Children are returned in arbitrary order (map iteration).
func WhichChildren(dest erl.Dest) ([]supervisor.ChildInfo, error) {
	return WhichChildrenTimeout(dest, DefaultAPITimeout)
}

// WhichChildrenTimeout is like [WhichChildren] but with a custom timeout.
func WhichChildrenTimeout(dest erl.Dest, tout time.Duration) ([]supervisor.ChildInfo, error) {
	self := erl.RootPID()

	result, err := genserver.Call(self, dest, whichChildrenRequest{}, tout)
	if err != nil {
		return nil, err
	}

	resp, ok := result.(whichChildrenResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected response type: %T", result)
	}

	return resp.children, nil
}

// CountChildren returns counts of children by category.
func CountChildren(dest erl.Dest) (supervisor.ChildCount, error) {
	return CountChildrenTimeout(dest, DefaultAPITimeout)
}

// CountChildrenTimeout is like [CountChildren] but with a custom timeout.
func CountChildrenTimeout(dest erl.Dest, tout time.Duration) (supervisor.ChildCount, error) {
	self := erl.RootPID()

	result, err := genserver.Call(self, dest, countChildrenRequest{}, tout)
	if err != nil {
		return supervisor.ChildCount{}, err
	}

	resp, ok := result.(countChildrenResponse)
	if !ok {
		return supervisor.ChildCount{}, fmt.Errorf("unexpected response type: %T", result)
	}

	return resp.count, nil
}

// defaultDynSup is the internal implementation used by StartDefaultLink.
type defaultDynSup struct {
	flags SupFlags
}

func (d defaultDynSup) Init(self erl.PID, args any) InitResult {
	return InitResult{SupFlags: d.flags}
}

// dynSupServer is the GenServer implementation for the dynamic supervisor.
// Stub methods satisfy the genserver.GenServer interface; full implementation in task 3.x.
type dynSupServer struct {
	callback DynamicSupervisor
}

// dynSupState holds the dynamic supervisor's internal state.
// Full fields will be added in task 3.x.
type dynSupState struct{}

// Interface compliance check.
var _ genserver.GenServer[dynSupState] = dynSupServer{}

func (s dynSupServer) Init(self erl.PID, args any) (genserver.InitResult[dynSupState], error) {
	panic("dynsup: Init not yet implemented")
}

func (s dynSupServer) HandleCall(self erl.PID, request any, from genserver.From, state dynSupState) (genserver.CallResult[dynSupState], error) {
	panic("dynsup: HandleCall not yet implemented")
}

func (s dynSupServer) HandleCast(self erl.PID, request any, state dynSupState) (genserver.CastResult[dynSupState], error) {
	return genserver.CastResult[dynSupState]{State: state}, nil
}

func (s dynSupServer) HandleInfo(self erl.PID, msg any, state dynSupState) (genserver.InfoResult[dynSupState], error) {
	panic("dynsup: HandleInfo not yet implemented")
}

func (s dynSupServer) HandleContinue(self erl.PID, continuation any, state dynSupState) (dynSupState, any, error) {
	return state, nil, nil
}

func (s dynSupServer) Terminate(self erl.PID, reason error, state dynSupState) {}
