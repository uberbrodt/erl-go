/*
Package dynsup provides an Elixir-style DynamicSupervisor for managing children
started at runtime.

Unlike [supervisor], which defines children at initialization, a dynamic supervisor
starts with zero children and adds them on demand via [StartChild]. Children are
identified by PID rather than string ID, and shutdown is concurrent (no ordering).

# Quick Start

The simplest way to create a dynamic supervisor is with [StartDefaultLink]:

	flags := dynsup.NewSupFlags(
		dynsup.SetIntensity(3),
		dynsup.SetPeriod(5),
		dynsup.SetMaxChildren(100),
	)

	supPID, err := dynsup.StartDefaultLink(self, flags)

Children are added at runtime using [StartChild]:

	spec := dynsup.NewChildSpec(func(sup erl.PID) (erl.PID, error) {
		return genserver.StartLink[WorkerState](sup, MyWorker{}, config)
	})

	childPID, err := dynsup.StartChild(supPID, spec)

And removed using [TerminateChild]:

	err := dynsup.TerminateChild(supPID, childPID)

# Supervision Tree Integration

A dynamic supervisor is typically a child of a regular [supervisor]. Use
[supervisor.SupervisorChild] type and infinite shutdown to allow the dynsup's
children to shut down gracefully:

	supervisor.NewChildSpec("pool_supervisor",
		func(sup erl.PID) (erl.PID, error) {
			flags := dynsup.NewSupFlags(dynsup.SetMaxChildren(50))
			return dynsup.StartDefaultLink(sup, flags)
		},
		supervisor.SetChildType(supervisor.SupervisorChild),
		supervisor.SetShutdown(supervisor.ShutdownOpt{Infinity: true}),
	)

# Child Restart Types

Each child specifies when it should be restarted via its [supervisor.ChildSpec]:

  - [supervisor.Permanent]: Always restart, regardless of exit reason (default).
  - [supervisor.Transient]: Restart only on abnormal exit.
  - [supervisor.Temporary]: Never restart; removed from supervisor on exit.

Use [supervisor.SetRestart] when creating child specs:

	spec := dynsup.NewChildSpec(workerStart,
		supervisor.SetRestart(supervisor.Transient),
	)

# MaxChildren

Use [SetMaxChildren] to cap the number of concurrent children. [StartChild]
returns [ErrMaxChildren] when the limit is reached:

	flags := dynsup.NewSupFlags(dynsup.SetMaxChildren(100))
	supPID, _ := dynsup.StartDefaultLink(self, flags)

	// After 100 children:
	_, err := dynsup.StartChild(supPID, spec)
	// err == dynsup.ErrMaxChildren

Set to 0 (the default) for unlimited children.

# Restart Intensity

Like [supervisor], the dynamic supervisor tracks restart frequency via
[SupFlags.Intensity] and [SupFlags.Period]. If more than Intensity restarts
occur within Period seconds, the supervisor terminates itself and all children,
propagating failure up the supervision tree.

# NewChildSpec

[NewChildSpec] is a convenience wrapper around [supervisor.NewChildSpec] that
omits the ID parameter (dynamic children are PID-keyed, not ID-keyed). Users
who already have a [supervisor.ChildSpec] can pass it directly to [StartChild].

# Custom Initialization

For cases requiring custom init logic, implement [DynamicSupervisor] and use
[StartLink]:

	type MyDynSup struct{}

	func (s MyDynSup) Init(self erl.PID, args any) dynsup.InitResult {
		config := args.(MyConfig)
		return dynsup.InitResult{
			SupFlags: dynsup.NewSupFlags(
				dynsup.SetMaxChildren(config.MaxWorkers),
			),
		}
	}

	supPID, err := dynsup.StartLink(self, MyDynSup{}, myConfig)

# Differences from Regular Supervisor

  - No strategy field: always one-for-one (only the failed child restarts)
  - Children keyed by PID, not string ID
  - No ordered startup/shutdown: children start/stop independently
  - No RestartChild/DeleteChild: [TerminateChild] removes the child entirely
  - Concurrent shutdown: all children terminate in parallel

See the Elixir DynamicSupervisor documentation for conceptual background:
https://hexdocs.pm/elixir/DynamicSupervisor.html
*/
package dynsup
