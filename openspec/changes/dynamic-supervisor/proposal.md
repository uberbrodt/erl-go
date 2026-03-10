## Why

The existing `supervisor` package supports static child lists defined at init time with unique IDs. Many concurrent systems need to dynamically scale workers at runtime — connection handlers, task workers, request processors — where the number of children isn't known ahead of time. This is the Elixir `DynamicSupervisor` / Erlang `simple_one_for_one` use case. The README already identifies this as a planned feature.

## What Changes

- New `erl/dynsup/` package implementing a dynamic supervisor with Elixir-style semantics
- Duplicate `childKiller` logic in `dynsup` (adapted for PID-keyed state and direct PID parameter instead of unexported ChildSpec fields)
- Public API: `StartLink`, `StartDefaultLink`, `StartChild` (full `supervisor.ChildSpec`), `TerminateChild` (by PID), `WhichChildren`, `CountChildren`, `NewChildSpec` (wrapper omitting ID parameter)
- Own `dynsup.SupFlags` type with `Period`, `Intensity`, `MaxChildren` (no `Strategy` field — always OneForOne implicitly) and own functional options (`NewSupFlags`, `SetPeriod`, `SetIntensity`, `SetMaxChildren`)
- Only `OneForOne` strategy (implicit, not configurable)
- Concurrent child shutdown (no ordering guarantees)
- `MaxChildren` option to cap the number of concurrent children
- Children are PID-keyed internally; `supervisor.ChildSpec.ID` is unused (null value)
- No `RestartChild` or `DeleteChild` — termination removes the child entirely

## Capabilities

### New Capabilities
- `dynamic-supervisor`: Runtime dynamic child management with Elixir DynamicSupervisor semantics — start/terminate children by PID, concurrent shutdown, max_children limit
### Modified Capabilities

(none — the existing supervisor package is unchanged)

## Impact

- **New package**: `erl/dynsup/`
- **Reused types**: `supervisor.ChildSpec`, `supervisor.StartFunSpec`, `supervisor.Restart`, `supervisor.ShutdownOpt`, `supervisor.ChildType`, `supervisor.ChildInfo`, `supervisor.ChildCount`
- **Duplicated logic**: `childKiller` from `supervisor` is adapted and duplicated in `dynsup` (circular dependency and unexported fields prevent extraction to a shared package)
- **No changes** to existing `supervisor` package
