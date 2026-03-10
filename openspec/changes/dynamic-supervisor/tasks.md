## 1. dynsup package types and scaffolding

- [ ] 1.1 Create `erl/dynsup/` package with `types.go`: `DynamicSupervisor` interface, `InitResult`, own `SupFlags` struct (Period, Intensity, MaxChildren — no Strategy field), `SupFlag` functional option type, `NewSupFlags`/`SetPeriod`/`SetIntensity`/`SetMaxChildren` option funcs, `ErrMaxChildren`, `ErrNotFound`, `DefaultAPITimeout`, internal request/response types for HandleCall
- [ ] 1.2 Create `erl/dynsup/doc.go` with stub package description (full examples added after API is implemented in task 6.1)
- [ ] 1.3 Add `dynsup.NewChildSpec(start, opts...)` wrapper that delegates to `supervisor.NewChildSpec("", start, opts...)`

## 2. dynsup child killer

- [ ] 2.1 Create `erl/dynsup/child_killer.go` — adapted from `supervisor/child_killer.go` to take child PID as a direct parameter (since `supervisor.ChildSpec.pid` is unexported), using `supervisor.ShutdownOpt` and `supervisor.Restart` for shutdown behavior

## 3. dynsup GenServer implementation

- [ ] 3.1 Implement `dynSupState` struct with `map[erl.PID]supervisor.ChildSpec`, `SupFlags`, restart intensity tracking. Implement no-op HandleCast and HandleContinue stubs (required by genserver.GenServer interface)
- [ ] 3.2 Implement `Init` callback: set TrapExit, call DynamicSupervisor.Init, handle Ignore, initialize empty children map
- [ ] 3.3 Implement `HandleCall` for `startChildRequest`: MaxChildren check, start child via spec.Start with defer/recover panic protection, insert into PID map
- [ ] 3.4 Implement `HandleCall` for `terminateChildRequest`: PID lookup, terminate via childKiller, remove from map
- [ ] 3.5 Implement `HandleCall` for `whichChildrenRequest` and `countChildrenRequest`: iterate PID map, build response types
- [ ] 3.6 Implement `HandleInfo` for `erl.ExitMsg`: PID lookup (silently ignore unrecognized PIDs), restart logic (Permanent/Transient/Temporary), intensity tracking, PID swap on restart. On restart failure, propagate error to terminate supervisor.
- [ ] 3.7 Implement `Terminate`: concurrent shutdown — spawn childKiller for all children, wait on shared channel

## 4. dynsup public API

- [ ] 4.1 Create `erl/dynsup/api.go` with `StartLink`, `StartDefaultLink`, `StartChild`/`StartChildTimeout`, `TerminateChild`/`TerminateChildTimeout`
- [ ] 4.2 Add `WhichChildren`/`WhichChildrenTimeout`, `CountChildren`/`CountChildrenTimeout` to api.go
- [ ] 4.3 Add `SetName` LinkOpt for named supervisor registration
- [ ] 4.4 Create internal `defaultDynSup` struct for `StartDefaultLink` (returns provided flags from Init)

## 5. Tests

- [ ] 5.1 Test StartLink: basic startup, named registration, Ignore from Init
- [ ] 5.2 Test StartDefaultLink: basic startup without callback
- [ ] 5.3 Test StartChild: successful start, MaxChildren enforcement, start failure, panic recovery, exitreason.Ignore handling
- [ ] 5.4 Test TerminateChild: successful termination, PID not found error
- [ ] 5.5 Test automatic restart: Permanent always restarts, Transient restarts on abnormal exit only, Temporary never restarts
- [ ] 5.6 Test restart failure: supervisor terminates when Start fails during automatic restart
- [ ] 5.7 Test restart intensity: supervisor terminates when intensity exceeded
- [ ] 5.8 Test ExitMsg from unmanaged process: silently ignored
- [ ] 5.9 Test WhichChildren and CountChildren: correct counts and info for mixed child types
- [ ] 5.10 Test concurrent shutdown: all children terminated during supervisor Terminate
- [ ] 5.11 Test NewChildSpec: creates spec with empty ID and correct defaults

## 6. Documentation

- [ ] 6.1 Flesh out `erl/dynsup/doc.go` with full examples: `StartDefaultLink` as primary API, supervision tree integration (dynsup as child of regular supervisor with `SetChildType(SupervisorChild)` and `SetShutdown(Infinity)`), basic StartChild/TerminateChild usage
