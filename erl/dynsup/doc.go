/*
Package dynsup provides an Elixir-style DynamicSupervisor for managing children
started at runtime.

Unlike [supervisor], which defines children at initialization, a dynamic supervisor
starts with zero children and adds them on demand via [StartChild]. Children are
identified by PID rather than string ID, and shutdown is concurrent (no ordering).

# Quick Start

The simplest way to create a dynamic supervisor is with StartDefaultLink:

	flags := dynsup.NewSupFlags(
		dynsup.SetIntensity(3),
		dynsup.SetPeriod(5),
		dynsup.SetMaxChildren(100),
	)

	supPID, err := dynsup.StartDefaultLink(self, flags)

For custom initialization logic, implement the [DynamicSupervisor] interface
and use StartLink.
*/
package dynsup
