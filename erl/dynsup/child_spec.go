package dynsup

import "github.com/uberbrodt/erl-go/erl/supervisor"

// NewChildSpec creates a [supervisor.ChildSpec] for use with a dynamic supervisor.
//
// This is a convenience wrapper around [supervisor.NewChildSpec] that omits the
// ID parameter (passes empty string), since dynamic supervisor children are
// identified by PID, not by string ID.
//
// Users who already have a [supervisor.ChildSpec] (e.g., from shared code) can
// pass it directly to [StartChild] without using this wrapper.
//
// Default values (inherited from [supervisor.NewChildSpec]):
//   - Restart: [supervisor.Permanent]
//   - Shutdown: 5000ms timeout
//   - Type: [supervisor.WorkerChild]
//
// Examples:
//
//	// Basic worker with defaults
//	spec := dynsup.NewChildSpec(func(sup erl.PID) (erl.PID, error) {
//		return genserver.StartLink[State](sup, MyServer{}, nil)
//	})
//
//	// Transient worker
//	spec := dynsup.NewChildSpec(workerStart,
//		supervisor.SetRestart(supervisor.Transient),
//	)
func NewChildSpec(start supervisor.StartFunSpec, opts ...supervisor.ChildSpecOpt) supervisor.ChildSpec {
	return supervisor.NewChildSpec("", start, opts...)
}
