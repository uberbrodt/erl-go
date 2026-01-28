/*
Package supervisor provides OTP-style process supervision for building fault-tolerant applications.

A supervisor is a process that manages child processes, automatically restarting
them according to a configured strategy when they exit. Supervisors can be nested
to form supervision trees, providing hierarchical fault isolation.

# Quick Start

The simplest way to create a supervisor is with [StartDefaultLink]:

	children := []supervisor.ChildSpec{
		supervisor.NewChildSpec("worker1", func(sup erl.PID) (erl.PID, error) {
			return genserver.StartLink[MyState](sup, MyServer{}, nil)
		}),
		supervisor.NewChildSpec("worker2", func(sup erl.PID) (erl.PID, error) {
			return genserver.StartLink[OtherState](sup, OtherServer{}, nil)
		}, supervisor.SetRestart(supervisor.Transient)),
	}

	supFlags := supervisor.NewSupFlags(
		supervisor.SetStrategy(supervisor.OneForOne),
		supervisor.SetIntensity(3),
		supervisor.SetPeriod(5),
	)

	supPID, err := supervisor.StartDefaultLink(self, children, supFlags)

For dynamic child configuration based on runtime arguments, implement the
[Supervisor] interface and use [StartLink].

# Restart Strategies

The supervisor's strategy determines which children are restarted when one fails:

  - [OneForOne]: Only the failed child is restarted. Other children are unaffected.
    Use when children are independent.

  - [OneForAll]: All children are terminated and restarted when one fails.
    Use when children are interdependent and cannot function without each other.

  - [RestForOne]: The failed child and all children started after it are restarted.
    Use when later children depend on earlier ones (e.g., workers depend on a
    connection pool started before them).

# Child Restart Types

Each child specifies when it should be restarted via [ChildSpec.Restart]:

  - [Permanent]: Always restart, regardless of exit reason. This is the default.
    Use for long-running services that should always be available.

  - [Transient]: Restart only on abnormal exit (not [exitreason.Normal],
    [exitreason.Shutdown], or [exitreason.SupervisorShutdown]).
    Use for tasks that should complete successfully but retry on failure.

  - [Temporary]: Never restart; child is removed from supervisor on exit.
    Does not count toward restart intensity. Use for one-off tasks.

# Restart Intensity

Supervisors track restart frequency via [SupFlagsS.Intensity] and [SupFlagsS.Period].
If more than Intensity restarts occur within Period seconds, the supervisor concludes
something is fundamentally wrong and terminates itself (and all children). This
prevents infinite restart loops and propagates failure up the supervision tree.

Default is 1 restart per 5 seconds. Set Intensity to 0 to terminate on the first
child failure.

# Shutdown Behavior

When a supervisor terminates (or restarts children due to strategy), it sends exit
signals to children and waits for them to terminate. The [ShutdownOpt] in each
[ChildSpec] controls this:

  - Timeout (default 5000ms): Wait up to N milliseconds, then force kill
  - BrutalKill: Immediately force kill without waiting
  - Infinity: Wait forever (recommended for supervisor children to allow their
    subtree to shut down gracefully)

# Supervision Trees

Supervisors can supervise other supervisors, forming a supervision tree. When
creating a nested supervisor as a child:

	supervisor.NewChildSpec("sub_supervisor",
		subSupStart,
		supervisor.SetChildType(supervisor.SupervisorChild),
		supervisor.SetShutdown(supervisor.ShutdownOpt{Infinity: true}),
	)

Use [SupervisorChild] type and Infinity shutdown to allow the child supervisor's
entire subtree to shut down gracefully.

# Child Start Function

The [StartFunSpec] function must link the child to the supervisor:

	func myChildStart(sup erl.PID) (erl.PID, error) {
		return genserver.StartLink[MyState](sup, MyServer{}, initArgs)
	}

Return values:
  - (pid, nil): Child started successfully
  - (_, [exitreason.Ignore]): Child intentionally not started; supervisor continues
  - (_, error): Child failed to start; supervisor fails and rolls back

Failing to link means the supervisor won't receive exit signals from the child.

# Dynamic Child Management

Supervisors support runtime child management via these APIs:

  - [StartChild]: Add and start a new child dynamically
  - [TerminateChild]: Stop a running child (keeping its spec for later restart)
  - [RestartChild]: Restart a previously terminated child
  - [DeleteChild]: Remove a child specification entirely
  - [WhichChildren]: List all children and their current status
  - [CountChildren]: Get counts of children by category

These APIs follow Erlang supervisor semantics. Dynamic operations do not
affect restart intensity calculations - only automatic restarts from child
exits count toward intensity limits.

Example:

	// Add a new child at runtime
	spec := supervisor.NewChildSpec("new_worker", workerStartFn)
	pid, err := supervisor.StartChild(supPID, spec)

	// Stop a child but keep its spec
	err = supervisor.TerminateChild(supPID, "new_worker")

	// Restart it later
	pid, err = supervisor.RestartChild(supPID, "new_worker")

	// Remove the spec entirely
	err = supervisor.TerminateChild(supPID, "new_worker")
	err = supervisor.DeleteChild(supPID, "new_worker")

	// Inspect children
	children, err := supervisor.WhichChildren(supPID)
	count, err := supervisor.CountChildren(supPID)

# Differences from Erlang/OTP

This implementation follows Erlang supervisor semantics closely, with these notes:

  - No simple_one_for_one strategy (planned: use DynamicSupervisor when available)

See the Erlang supervisor documentation for conceptual background:
https://www.erlang.org/doc/apps/stdlib/supervisor
*/
package supervisor
