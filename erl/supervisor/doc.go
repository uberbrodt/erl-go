/*
Package supervisor provides OTP-style process supervision for building fault-tolerant applications.

A supervisor is a process that manages child processes, automatically restarting
them according to a configured strategy when they exit. Supervisors can be nested
to form supervision trees, providing hierarchical fault isolation.

# Restart Strategies

The supervisor's strategy determines which children are restarted when one fails:

  - [OneForOne]: Only the failed child is restarted. Other children are unaffected.
    Use when children are independent.
  - [OneForAll]: All children are terminated and restarted when one fails.
    Use when children are interdependent and cannot function without each other.
  - [RestForOne]: The failed child and all children started after it are restarted.
    Use when later children depend on earlier ones.

# Child Restart Types

Each child specifies when it should be restarted:

  - [Permanent]: Always restart, regardless of exit reason (default)
  - [Transient]: Restart only on abnormal exit (not Normal/Shutdown)
  - [Temporary]: Never restart; child is removed from supervisor on exit

# Restart Intensity

Supervisors track restart frequency via Intensity and Period settings. If more than
Intensity restarts occur within Period seconds, the supervisor concludes something is
fundamentally wrong and terminates itself (and all children). This prevents infinite
restart loops. Default is 1 restart per 5 seconds.

# Basic Usage

Start a supervisor with a static list of children using [StartDefaultLink]:

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

For dynamic child configuration, implement the [Supervisor] interface and use [StartLink].

# Shutdown Behavior

When a supervisor terminates (or restarts children), it sends exit signals to children
and waits for them to terminate. The [ShutdownOpt] in each [ChildSpec] controls this:

  - Timeout (default 5000ms): Wait up to N milliseconds, then force kill
  - BrutalKill: Immediately force kill without waiting
  - Infinity: Wait forever (recommended for supervisor children)

# Differences from Erlang/OTP

This implementation follows Erlang supervisor semantics closely, with these notes:

  - No simple_one_for_one strategy (use a regular supervisor with dynamic children)
  - No start_child/terminate_child/restart_child dynamic API yet
  - No count_children/which_children introspection API yet

See the Erlang supervisor documentation for conceptual background:
https://www.erlang.org/doc/apps/stdlib/supervisor
*/
package supervisor

