package dynsup

import (
	"errors"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/supervisor"
	"github.com/uberbrodt/erl-go/erl/timeout"
)

// DefaultAPITimeout is the default timeout for dynamic supervisor API calls.
// Individual calls can use the *Timeout variants for custom timeouts.
var DefaultAPITimeout = timeout.Default

// Errors returned by dynamic supervisor APIs.
// Use [errors.Is] to check for specific error conditions.
var (
	// ErrMaxChildren is returned by [StartChild] when the dynamic supervisor
	// has reached its MaxChildren limit and cannot start additional children.
	ErrMaxChildren = errors.New("max children reached")

	// ErrNotFound is returned by [TerminateChild] when the given PID is not
	// a child of this dynamic supervisor.
	ErrNotFound = errors.New("child not found")
)

// SupFlags configures dynamic supervisor behavior including restart intensity
// limits and maximum child count.
//
// Unlike [supervisor.SupFlagsS], there is no Strategy field — dynamic supervisors
// always use one-for-one semantics (only the failed child is restarted).
//
// Create using [NewSupFlags] with functional options:
//
//	flags := dynsup.NewSupFlags(
//		dynsup.SetIntensity(3),
//		dynsup.SetPeriod(10),
//		dynsup.SetMaxChildren(100),
//	)
//
// The default values (Period=5, Intensity=1, MaxChildren=0) provide conservative
// restart protection with no child limit.
type SupFlags struct {
	// Period is the time window in seconds for counting restarts.
	// Restarts older than Period seconds are not counted toward intensity.
	//
	// Default: 5 seconds
	Period int

	// Intensity is the maximum number of restarts allowed within Period seconds.
	// If this limit is exceeded, the supervisor terminates itself and all children,
	// propagating the failure up the supervision tree.
	//
	// Default: 1
	Intensity int

	// MaxChildren is the maximum number of children allowed. StartChild returns
	// [ErrMaxChildren] when this limit is reached. A value of 0 means unlimited.
	//
	// Default: 0 (unlimited)
	MaxChildren int
}

// SupFlag is a functional option for configuring [SupFlags].
// Use with [NewSupFlags].
type SupFlag func(flags SupFlags) SupFlags

// SetPeriod sets the restart evaluation window in seconds.
//
// The supervisor tracks restarts within this rolling window. Restarts older
// than Period seconds are forgotten and don't count toward intensity.
//
// Default: 5 seconds
func SetPeriod(period int) SupFlag {
	return func(flags SupFlags) SupFlags {
		flags.Period = period
		return flags
	}
}

// SetIntensity sets the maximum number of restarts allowed within the period.
//
// If more than Intensity restarts occur within Period seconds, the supervisor
// terminates itself (and all children), propagating the failure up the
// supervision tree.
//
// Default: 1
func SetIntensity(intensity int) SupFlag {
	return func(flags SupFlags) SupFlags {
		flags.Intensity = intensity
		return flags
	}
}

// SetMaxChildren sets the maximum number of children allowed.
//
// When the limit is reached, [StartChild] returns [ErrMaxChildren].
// Set to 0 for unlimited children (the default).
//
// Default: 0 (unlimited)
func SetMaxChildren(max int) SupFlag {
	return func(flags SupFlags) SupFlags {
		flags.MaxChildren = max
		return flags
	}
}

// NewSupFlags creates dynamic supervisor flags with the given options.
//
// Default values:
//   - Period: 5 seconds
//   - Intensity: 1 restart
//   - MaxChildren: 0 (unlimited)
//
// Examples:
//
//	// Use defaults (1 restart per 5 seconds, unlimited children)
//	flags := dynsup.NewSupFlags()
//
//	// Custom configuration
//	flags := dynsup.NewSupFlags(
//		dynsup.SetIntensity(3),
//		dynsup.SetPeriod(10),
//		dynsup.SetMaxChildren(100),
//	)
func NewSupFlags(flags ...SupFlag) SupFlags {
	f := SupFlags{
		Period:    5,
		Intensity: 1,
	}

	for _, x := range flags {
		f = x(f)
	}
	return f
}

// InitResult is returned by the [DynamicSupervisor.Init] callback to configure
// the dynamic supervisor.
//
// Unlike [supervisor.InitResult], there are no ChildSpecs — dynamic supervisors
// start with zero children. Children are added at runtime via [StartChild].
type InitResult struct {
	// SupFlags configures the supervisor's restart intensity limits and child cap.
	// Use [NewSupFlags] to create with defaults and customize as needed.
	SupFlags SupFlags

	// Ignore, if true, causes the supervisor to exit with [exitreason.Ignore],
	// preventing it from starting.
	Ignore bool
}

// DynamicSupervisor is the callback interface for dynamic supervisor configuration.
//
// Implement this interface when you need custom initialization logic.
// For most cases, use [StartDefaultLink] instead which doesn't require
// implementing this interface.
//
// Example:
//
//	type MyDynSup struct{}
//
//	func (s MyDynSup) Init(self erl.PID, args any) dynsup.InitResult {
//		return dynsup.InitResult{
//			SupFlags: dynsup.NewSupFlags(
//				dynsup.SetMaxChildren(50),
//			),
//		}
//	}
//
//	// Usage:
//	supPID, err := dynsup.StartLink(self, MyDynSup{}, nil)
type DynamicSupervisor interface {
	// Init is called when the dynamic supervisor starts to obtain configuration.
	//
	// Parameters:
	//   - self: The supervisor's own PID
	//   - args: Arguments passed to [StartLink]
	//
	// Return the supervisor flags in [InitResult].
	// Set Ignore=true to cancel supervisor startup.
	Init(self erl.PID, args any) InitResult
}

// Internal request types for HandleCall — used by public API functions.
type (
	startChildRequest     struct{ spec supervisor.ChildSpec }
	terminateChildRequest struct{ pid erl.PID }
	whichChildrenRequest  struct{}
	countChildrenRequest  struct{}
)

// Internal response types for HandleCall — returned to public API functions.
type (
	startChildResponse struct {
		pid erl.PID
		err error
	}
	terminateChildResponse struct{ err error }
	whichChildrenResponse  struct{ children []supervisor.ChildInfo }
	countChildrenResponse  struct{ count supervisor.ChildCount }
)
