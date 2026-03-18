## ADDED Requirements

### Requirement: DynamicSupervisor callback interface
The `dynsup` package SHALL define a `DynamicSupervisor` interface with an `Init(self erl.PID, args any) InitResult` method. The `InitResult` SHALL contain `SupFlags` (with `MaxChildren` and restart intensity/period configuration) and an `Ignore` boolean. The `InitResult` SHALL NOT contain child specs — dynamic supervisors start with zero children.

#### Scenario: Init returns configuration
- **WHEN** a DynamicSupervisor callback's Init is invoked
- **THEN** it SHALL return an InitResult with SupFlags configuration and no child specs

#### Scenario: Init returns Ignore
- **WHEN** a DynamicSupervisor callback's Init returns InitResult with Ignore=true
- **THEN** the supervisor SHALL exit with exitreason.Ignore and not start

### Requirement: StartLink creates a dynamic supervisor
The `dynsup` package SHALL provide `StartLink(self erl.PID, callback DynamicSupervisor, args any, ...LinkOpts) (erl.PID, error)` to create and link a dynamic supervisor process. The supervisor SHALL set TrapExit=true to receive child exit signals as ExitMsg. An optional `SetName` LinkOpt SHALL register the supervisor under a name.

#### Scenario: Successful startup
- **WHEN** StartLink is called with a valid callback
- **THEN** a new supervisor process SHALL be created, linked to self, with TrapExit enabled and zero children

#### Scenario: Named supervisor
- **WHEN** StartLink is called with SetName("my_dynsup")
- **THEN** the supervisor SHALL be registered under the name "my_dynsup" and be addressable via erl.WhereIs

### Requirement: StartDefaultLink convenience function
The `dynsup` package SHALL provide `StartDefaultLink(self erl.PID, flags SupFlags, ...LinkOpts) (erl.PID, error)` as a convenience that creates a dynamic supervisor without requiring a custom `DynamicSupervisor` callback implementation. Internally it SHALL create a default callback that returns the provided flags.

#### Scenario: Simple startup without callback
- **WHEN** StartDefaultLink is called with SupFlags
- **THEN** a new supervisor process SHALL be created with those flags and zero children, behaving identically to StartLink with a callback that returns those flags

### Requirement: NewChildSpec convenience constructor
The `dynsup` package SHALL provide `NewChildSpec(start supervisor.StartFunSpec, opts ...supervisor.ChildSpecOpt) supervisor.ChildSpec` as a wrapper that creates a `supervisor.ChildSpec` without requiring a child ID. Internally it SHALL call `supervisor.NewChildSpec("", start, opts...)`.

#### Scenario: Create child spec without ID
- **WHEN** dynsup.NewChildSpec is called with a start function and options
- **THEN** it SHALL return a supervisor.ChildSpec with an empty ID and the provided start function and options applied

### Requirement: Own SupFlags type without Strategy
The `dynsup` package SHALL define its own `SupFlags` struct with `Period` (int), `Intensity` (int), and `MaxChildren` (int) fields. It SHALL NOT include a `Strategy` field. The package SHALL provide `NewSupFlags(...SupFlag) SupFlags` constructor and functional options `SetPeriod`, `SetIntensity`, `SetMaxChildren`. Default values SHALL be: Period=5, Intensity=1, MaxChildren=0 (unlimited).

#### Scenario: Create SupFlags with defaults
- **WHEN** dynsup.NewSupFlags() is called with no options
- **THEN** it SHALL return SupFlags{Period: 5, Intensity: 1, MaxChildren: 0}

#### Scenario: Create SupFlags with MaxChildren
- **WHEN** dynsup.NewSupFlags(dynsup.SetMaxChildren(100)) is called
- **THEN** it SHALL return SupFlags with MaxChildren=100 and other fields at defaults

### Requirement: StartChild adds and starts a child dynamically
The `dynsup` package SHALL provide `StartChild(sup erl.Dest, spec supervisor.ChildSpec) (erl.PID, error)`. The ChildSpec's ID field SHALL be ignored. The child SHALL be started by calling spec.Start(sup) and tracked in a PID-keyed map. A `StartChildTimeout` variant SHALL accept a custom timeout.

#### Scenario: Successful child start
- **WHEN** StartChild is called with a valid ChildSpec
- **THEN** the child process SHALL be started, linked to the supervisor, and its PID returned

#### Scenario: Child start returns exitreason.Ignore
- **WHEN** StartChild is called and the child's Start function returns exitreason.Ignore
- **THEN** StartChild SHALL return `exitreason.Ignore` as the error and the child SHALL NOT be tracked

#### Scenario: Child start panics
- **WHEN** StartChild is called and the child's Start function panics
- **THEN** the panic SHALL be recovered, converted to `exitreason.Exception`, and returned as an error; the child SHALL NOT be tracked

#### Scenario: Child start fails
- **WHEN** StartChild is called and the child's Start function returns an error
- **THEN** StartChild SHALL return that error wrapped in `exitreason.Exception` and the child SHALL NOT be tracked

#### Scenario: MaxChildren exceeded
- **WHEN** StartChild is called and the number of active children equals MaxChildren (when MaxChildren > 0)
- **THEN** StartChild SHALL return ErrMaxChildren without starting the child

### Requirement: TerminateChild stops a child by PID
The `dynsup` package SHALL provide `TerminateChild(sup erl.Dest, pid erl.PID) error`. The child SHALL be terminated according to its ShutdownOpt and removed from the supervisor entirely. A `TerminateChildTimeout` variant SHALL accept a custom timeout.

#### Scenario: Successful termination
- **WHEN** TerminateChild is called with a PID of a running child
- **THEN** the child SHALL be terminated and removed from the supervisor's children map

#### Scenario: PID not found
- **WHEN** TerminateChild is called with a PID not managed by this supervisor
- **THEN** TerminateChild SHALL return ErrNotFound

### Requirement: Automatic restart on child exit
When a child exits, the supervisor SHALL handle the exit signal based on the child's Restart type. Permanent children SHALL always be restarted. Transient children SHALL be restarted only on abnormal exit (not Normal, Shutdown, or SupervisorShutdown). Temporary children SHALL be removed without restart. Restarts SHALL count toward restart intensity; if intensity is exceeded, the supervisor SHALL terminate.

#### Scenario: Permanent child crashes
- **WHEN** a Permanent child exits with any reason
- **THEN** the supervisor SHALL restart the child using its Start function, remove the old PID entry, and insert the new PID entry

#### Scenario: Transient child exits normally
- **WHEN** a Transient child exits with exitreason.Normal or exitreason.Shutdown
- **THEN** the supervisor SHALL remove the child without restarting

#### Scenario: Transient child crashes
- **WHEN** a Transient child exits with an abnormal reason (Exception, Kill)
- **THEN** the supervisor SHALL restart the child

#### Scenario: Temporary child exits
- **WHEN** a Temporary child exits with any reason
- **THEN** the supervisor SHALL remove the child without restarting and without counting toward restart intensity

#### Scenario: Restart attempt fails
- **WHEN** a child exit triggers a restart and the Start function returns an error
- **THEN** the supervisor SHALL terminate itself, propagating the failure up the supervision tree. The failed restart attempt SHALL count toward restart intensity.

#### Scenario: Restart intensity exceeded
- **WHEN** the number of restarts within the configured Period exceeds the configured Intensity
- **THEN** the supervisor SHALL terminate itself, propagating the failure up the supervision tree

#### Scenario: ExitMsg from unmanaged process
- **WHEN** the supervisor receives an ExitMsg with a PID not in its children map (e.g., from a linked parent or an already-terminated child)
- **THEN** the supervisor SHALL silently ignore the message and continue operating

### Requirement: WhichChildren returns child information
The `dynsup` package SHALL provide `WhichChildren(sup erl.Dest) ([]supervisor.ChildInfo, error)`. Each ChildInfo SHALL have an empty ID (since dynamic children are not named), the child's PID, Type, Status, and Restart type. Order is non-deterministic. A `WhichChildrenTimeout` variant SHALL accept a custom timeout.

#### Scenario: List running children
- **WHEN** WhichChildren is called on a supervisor with active children
- **THEN** it SHALL return a ChildInfo for each child with Status=ChildRunning and the child's PID

### Requirement: CountChildren returns child counts
The `dynsup` package SHALL provide `CountChildren(sup erl.Dest) (supervisor.ChildCount, error)`. It SHALL return Specs (total tracked), Active (currently running), Workers, and Supervisors counts. A `CountChildrenTimeout` variant SHALL accept a custom timeout.

#### Scenario: Count with mixed child types
- **WHEN** CountChildren is called on a supervisor with 3 workers and 1 supervisor child
- **THEN** it SHALL return Specs=4, Active=4, Workers=3, Supervisors=1

### Requirement: Concurrent shutdown
When the dynamic supervisor terminates (via Terminate callback), it SHALL terminate all children concurrently rather than sequentially. Each child SHALL be terminated according to its ShutdownOpt. The supervisor SHALL wait for all children to complete termination before exiting.

#### Scenario: Shutdown with multiple children
- **WHEN** the dynamic supervisor's Terminate is called with N active children
- **THEN** all N children SHALL be terminated concurrently and the supervisor SHALL wait for all to finish

### Requirement: MaxChildren configuration
The `SupFlags` type SHALL include a `MaxChildren` field (int). When set to 0 or unset, there SHALL be no limit. When set to a positive value, `StartChild` SHALL return `ErrMaxChildren` if the limit would be exceeded.

#### Scenario: MaxChildren enforced
- **WHEN** MaxChildren is set to 5 and 5 children are running
- **THEN** the next StartChild call SHALL return ErrMaxChildren

#### Scenario: MaxChildren unlimited
- **WHEN** MaxChildren is 0 (default)
- **THEN** StartChild SHALL not enforce any child count limit
