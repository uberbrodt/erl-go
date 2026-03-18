## Context

erl-go's `supervisor` package implements Erlang-style static supervision: children are defined at init, identified by unique string IDs, and managed in ordered lists. The `childKiller` process handles graceful shutdown of individual children with timeout/brutal-kill/infinity semantics.

Dynamic workloads (connection handlers, task pools, request processors) need children started and stopped at runtime without predefined IDs or ordering. Elixir solved this by splitting `DynamicSupervisor` out as a separate module from the regular `Supervisor`.

The existing supervisor already has `StartChild`/`TerminateChild` APIs, but these operate on named specs and maintain ordering — they're for adding known children to a static tree, not for managing pools of ephemeral workers.

## Goals / Non-Goals

**Goals:**
- Provide a `dynsup` package with Elixir DynamicSupervisor semantics
- Support runtime `StartChild` with full `supervisor.ChildSpec` (Elixir model)
- Support `TerminateChild` by PID (not by string ID)
- Implement concurrent shutdown (no ordering guarantees)
- Support `MaxChildren` to cap concurrent children
- Reuse existing types from `supervisor` where appropriate (`ChildSpec`, `Restart`, `ShutdownOpt`, etc.)
- Provide `StartDefaultLink` convenience for simple cases (no callback needed)

**Non-Goals:**
- Template/factory model (Erlang `simple_one_for_one` style) — using full-spec model instead
- `RestartChild` or `DeleteChild` APIs — terminate removes the child entirely
- `OneForAll` or `RestForOne` strategies — only `OneForOne` applies to dynamic children
- Ordered shutdown — children are independent, shutdown is concurrent
- Named/registered children — children are PID-keyed only

## Decisions

### 1. Separate `dynsup` package (not extending `supervisor`)

**Choice**: New `erl/dynsup/` package.

**Alternatives considered**:
- Add a `DynamicSupervisor` mode to the existing `supervisor` package
- Use a strategy constant like `SimpleOneForOne`

**Rationale**: The behavioral differences are significant enough that combining them would add complexity to the existing supervisor with many conditional paths. Elixir made the same choice. Separate packages keep each focused and testable.

### 2. Full child spec model (Elixir) over template model (Erlang)

**Choice**: `StartChild` accepts a complete `supervisor.ChildSpec`.

**Alternatives considered**:
- Template with extra args (Erlang `simple_one_for_one` style)
- Factory function that produces specs from args

**Rationale**: Go closures naturally parameterize start functions — callers capture config in the closure passed to `NewChildSpec`. A template model adds indirection without benefit. Full specs also allow different restart types per child if needed.

### 3. PID-keyed `map[erl.PID]supervisor.ChildSpec` for internal state

**Choice**: Use a map keyed by PID instead of the ordered `childSpecs` list.

**Rationale**: Dynamic children have no meaningful ordering. PID lookup is the primary operation (exit signal handling, terminate by PID). A map gives O(1) for both. The regular supervisor's `childSpecs` container is built around ordered lists and ID-based lookup — neither applies here.

### 4. Duplicate `childKiller` in `dynsup` (not extracted to shared package)

**Choice**: Duplicate and adapt the `childKiller` logic in `dynsup`.

**Alternatives considered**:
- Extract to `erl/internal/suputil/` shared package
- Export `childKiller` from `supervisor`

**Rationale**: Extraction creates a circular dependency problem — `childKiller` references `supervisor.ChildSpec` unexported fields (`pid`, `Restart`) and `supervisor` would need to import the shared package. The options to resolve this (move shared types to suputil + re-export via aliases, or use raw parameters) add significant complexity for 170 lines of stable logic. The `dynsup` version is adapted anyway: it takes the child PID as a direct parameter (since `ChildSpec.pid` is unexported) and uses `supervisor.Restart` and `supervisor.ShutdownOpt` from the imported package.

### 5. ChildSpec unexported fields constraint

**Choice**: `dynsup` does not access `ChildSpec.pid`, `.ignored`, or `.terminated` — these are unexported fields only usable within the `supervisor` package.

**Rationale**: `dynsup` doesn't need them. The PID is the map key (not stored on the spec). There's no "terminated" state — terminated children are removed from the map entirely. There's no "ignored" state — if `Start` returns `exitreason.Ignore`, the child is never added. The `ChildSpec` stored in the map is used only for its exported fields: `Start`, `Restart`, `Shutdown`, `Type`.

### 6. Reuse `supervisor.ChildSpec` with `dynsup.NewChildSpec` wrapper

**Choice**: Reuse the existing `supervisor.ChildSpec` type, but provide `dynsup.NewChildSpec(start, opts...)` that omits the ID parameter (passes empty string internally).

**Alternatives considered**:
- Define a new `dynsup.ChildSpec` without the ID field
- Auto-generate IDs
- Have users call `supervisor.NewChildSpec("", start, opts...)` directly

**Rationale**: Reusing the type avoids duplication and lets users access the same `ChildSpecOpt` functions (`SetRestart`, `SetShutdown`, etc.). But requiring `supervisor.NewChildSpec("")` at every call site is ugly and signals "this API wasn't designed for you." The `dynsup.NewChildSpec` wrapper eliminates the vestigial ID parameter while delegating to `supervisor.NewChildSpec` internally. Users who already have a `supervisor.ChildSpec` (e.g., from shared code) can still pass it to `StartChild` directly.

### 7. Own `dynsup.SupFlags` type (not reusing `supervisor.SupFlagsS`)

**Choice**: Define a separate `dynsup.SupFlags` struct with only `Period`, `Intensity`, and `MaxChildren`. Provide own functional options: `NewSupFlags`, `SetPeriod`, `SetIntensity`, `SetMaxChildren`.

**Alternatives considered**:
- Embed `supervisor.SupFlagsS` and add MaxChildren
- Type-alias `supervisor.SupFlagsS`

**Rationale**: `supervisor.SupFlagsS` has a `Strategy` field that is meaningless for dynsup (always OneForOne). Exposing it creates confusion — users see it in autocomplete/docs and wonder if they should set it. A clean struct with only the relevant fields makes the API self-documenting. The cost is duplicating `SetPeriod`/`SetIntensity` option funcs (~20 lines), which is trivial.

### 8. StartDefaultLink as the primary API

**Choice**: Provide `StartDefaultLink(self, flags, ...LinkOpts)` in addition to `StartLink(self, callback, args, ...LinkOpts)`. Lead all documentation and examples with `StartDefaultLink`.

**Rationale**: A dynamic supervisor starts with zero children, so Init rarely needs custom logic. `StartDefaultLink` covers 90%+ of use cases. The `DynamicSupervisor` callback interface exists for cases where the supervisor's own PID is needed during initialization or conditional startup logic. Docs should make `StartDefaultLink` feel like the default path and `StartLink` the advanced one.

### 9. Panic recovery in child start

**Choice**: `StartChild` wraps the `spec.Start(sup)` call with `defer/recover` to catch panics and convert them to `exitreason.Exception`.

**Rationale**: Matches the regular supervisor's behavior (supervisor.go:524-539). User-provided `Start` functions may panic, and without recovery the entire dynamic supervisor process would crash instead of returning an error to the caller.

### 10. Concurrent shutdown

**Choice**: Terminate all children concurrently during supervisor shutdown.

**Rationale**: Dynamic children have no ordering dependencies. Concurrent shutdown is faster and matches Elixir behavior. Implementation: spawn all `childKiller` processes, then collect results from a shared channel.

### 11. GenServer-based implementation

**Choice**: `dynsup` internally implements `genserver.GenServer[dynSupState]`, same pattern as the regular supervisor.

**Rationale**: Consistent with existing architecture. TrapExit for child exit signals, HandleCall for API operations, HandleInfo for exit messages. Proven pattern.

## Risks / Trade-offs

**[Map iteration order is non-deterministic]** → `WhichChildren` returns children in arbitrary order. This is acceptable since dynamic children have no meaningful ordering. Document this.

**[ChildSpec.ID field is vestigial in dynsup]** → Could confuse users who set it expecting it to matter. Document that ID is ignored for dynamic children. Consider logging a warning if ID is non-empty.

**[Concurrent shutdown may overwhelm system]** → If a dynsup has thousands of children, spawning that many `childKiller` processes simultaneously could spike resource usage. For now this matches Elixir behavior and is acceptable. Could add batched shutdown later if needed.

**[Restart replaces PID in map]** → When a Permanent/Transient child is restarted after crash, the old PID entry must be deleted and a new one inserted. Race window: between child exit and restart completion, the child count temporarily drops. This is the same semantic as Elixir and is expected.

**[TerminateChild + stale ExitMsg ordering]** → When `TerminateChild` is called via HandleCall, the childKiller unlinks the child before killing it. However, an ExitMsg may already be queued in the supervisor's inbox before the unlink takes effect. This is safe because: (1) HandleCall runs to completion before HandleInfo processes the next message (GenServer serial processing guarantee), (2) the PID is deleted from the map during HandleCall, (3) when the stale ExitMsg arrives in HandleInfo, the map lookup misses and it is silently dropped. PID reuse cannot cause a false match because `erl.PID` contains a `*Process` pointer, and the ExitMsg holds a reference to the old Process (preventing GC/reuse).

**[Restart failure terminates supervisor]** → If a Permanent/Transient child crashes and the restart attempt fails (Start returns error), the error propagates from HandleInfo, causing the supervisor to terminate. This matches Erlang/Elixir behavior. A failed restart attempt still counts toward restart intensity.
