# Supervisors

A supervisor is a process that supervises other processes, which we refer to as child processes. Supervisors are used to build a hierarchical process structure called a supervision tree. A supervision tree is a great way to structure a fault-tolerant application.

The supervisor package implements OTP-style process supervision. When a child process terminates, the supervisor automatically restarts it according to configured rules, isolating failures and maintaining system availability.

## Core Concepts

### Supervisor

A supervisor is a specialized `genserver` that manages a list of child processes. It's responsible for:

- **Starting** children in a defined order during initialization
- **Monitoring** children for termination via linked exit signals
- **Restarting** children according to the configured strategy
- **Shutting down** children gracefully when the supervisor terminates
- **Preventing restart loops** by tracking restart frequency

### ChildSpec

A `ChildSpec` defines how a supervisor manages a child process. Create one using `NewChildSpec`:

```go
spec := supervisor.NewChildSpec("my_worker", startFn,
    supervisor.SetRestart(supervisor.Transient),
    supervisor.SetShutdown(supervisor.ShutdownOpt{Timeout: 10_000}),
    supervisor.SetChildType(supervisor.WorkerChild),
)
```

Fields:

| Field | Description | Default |
|-------|-------------|---------|
| **ID** | Unique identifier within the supervisor. Used in logs and for `RestForOne` strategy. | (required) |
| **Start** | Function to start the child. Must link to supervisor. | (required) |
| **Restart** | When to restart: `Permanent`, `Transient`, or `Temporary` | `Permanent` |
| **Shutdown** | How to terminate: timeout, brutal kill, or infinity | 5000ms timeout |
| **Type** | `WorkerChild` or `SupervisorChild` (informational) | `WorkerChild` |

### StartFunSpec

The start function receives the supervisor's PID and **must link the child to it**:

```go
func myWorkerStart(sup erl.PID) (erl.PID, error) {
    return genserver.StartLink[MyState](sup, MyServer{}, initArgs)
}
```

Return values:
- `(pid, nil)` - Child started successfully
- `(_, exitreason.Ignore)` - Child intentionally not started; supervisor continues
- `(_, error)` - Child failed to start; supervisor fails and rolls back

**Important:** Failing to link the child means the supervisor won't receive exit signals and cannot supervise the process.

## Restart Strategies

The supervisor's restart strategy determines which children are restarted when one terminates.

### OneForOne (Default)

Only the terminated child is restarted. Other children are unaffected.

```
Children: [A] [B] [C]
B terminates → Restart B only
Result:   [A] [B'] [C]
```

**Use when:** Children are independent and don't rely on each other.

### OneForAll

All children are terminated and restarted (in start order) when any single child terminates.

```
Children: [A] [B] [C]
B terminates → Stop C, B, A → Start A, B, C
Result:   [A'] [B'] [C']
```

**Use when:** Children are tightly coupled and cannot function correctly if one fails. For example, a database connection pool and workers that depend on it.

### RestForOne

The terminated child and all children started **after** it are terminated, then restarted in order.

```
Children: [A] [B] [C]  (started in order A→B→C)
B terminates → Stop C, B → Start B, C
Result:   [A] [B'] [C']
```

**Use when:** Later children depend on earlier ones. For example, a connection manager (started first) that worker processes (started later) depend on.

## Restart Types

Each child specifies when it should be restarted via the `Restart` field.

### Permanent (Default)

Always restarted, regardless of exit reason.

```go
supervisor.NewChildSpec("worker", startFn,
    supervisor.SetRestart(supervisor.Permanent),
)
```

**Use for:** Long-running services that should always be available.

### Transient

Restarted only on **abnormal** exit. Not restarted if the exit reason is:
- `exitreason.Normal`
- `exitreason.Shutdown(...)`
- `exitreason.SupervisorShutdown`

```go
supervisor.NewChildSpec("task", startFn,
    supervisor.SetRestart(supervisor.Transient),
)
```

**Use for:** Tasks that should complete successfully but retry on failure.

### Temporary

Never restarted. Removed from supervisor on any exit. Does not count toward restart intensity.

```go
supervisor.NewChildSpec("one_shot", startFn,
    supervisor.SetRestart(supervisor.Temporary),
)
```

**Use for:** One-off tasks or children managed by external logic.

### Restart Decision Matrix

| Restart Type | Normal | Shutdown | SupervisorShutdown | Exception/Other |
|--------------|--------|----------|--------------------| ----------------|
| Permanent    | Restart | Restart | Restart | Restart |
| Transient    | Remove | Remove | Remove | Restart |
| Temporary    | Remove | Remove | Remove | Remove |

## Shutdown Options

When a supervisor terminates a child (during shutdown or restart), it sends an exit signal and waits according to the child's `ShutdownOpt`:

### Timeout (Default: 5000ms)

Wait up to N milliseconds for graceful shutdown. If the child doesn't terminate within the timeout, send a kill signal.

```go
supervisor.SetShutdown(supervisor.ShutdownOpt{Timeout: 10_000}) // 10 seconds
```

### BrutalKill

Immediately send a kill signal without waiting for graceful shutdown. The child's `Terminate` callback may not complete.

```go
supervisor.SetShutdown(supervisor.ShutdownOpt{BrutalKill: true})
```

**Use sparingly.** Only when the child cannot be trusted to shut down cleanly.

### Infinity

Wait indefinitely for the child to terminate. **Recommended for supervisor children** to allow their entire subtree to shut down gracefully.

```go
supervisor.SetShutdown(supervisor.ShutdownOpt{Infinity: true})
```

## Supervisor Flags

Supervisor flags control global behavior via `NewSupFlags`:

```go
supFlags := supervisor.NewSupFlags(
    supervisor.SetStrategy(supervisor.OneForOne), // Restart strategy
    supervisor.SetIntensity(3),                   // Max restarts
    supervisor.SetPeriod(5),                      // Within N seconds
)
```

### Restart Intensity

The supervisor tracks restart frequency to prevent infinite restart loops:

- **Period**: Time window in seconds (default: 5)
- **Intensity**: Maximum restarts allowed within the period (default: 1)

If more than `Intensity` restarts occur within `Period` seconds, the supervisor concludes something is fundamentally wrong and **terminates itself** (along with all children). This propagates the failure up the supervision tree.

**Example:** With `Intensity=3` and `Period=10`, the supervisor allows up to 3 restarts within any 10-second window. The 4th restart within 10 seconds causes the supervisor to terminate.

Setting `Intensity=0` means the supervisor terminates on the first child restart.

## Usage Examples

### Static Children

Use `StartDefaultLink` when children are known at compile time:

```go
package main

import (
    "github.com/uberbrodt/erl-go/erl"
    "github.com/uberbrodt/erl-go/erl/genserver"
    "github.com/uberbrodt/erl-go/erl/supervisor"
)

func main() {
    rootPID := erl.Spawn(func(self erl.PID, inbox <-chan any) error {
        // Define children
        children := []supervisor.ChildSpec{
            supervisor.NewChildSpec("database", func(sup erl.PID) (erl.PID, error) {
                return genserver.StartLink[DBState](sup, DBServer{}, dbConfig)
            }),
            supervisor.NewChildSpec("cache", func(sup erl.PID) (erl.PID, error) {
                return genserver.StartLink[CacheState](sup, CacheServer{}, nil)
            }, supervisor.SetRestart(supervisor.Transient)),
            supervisor.NewChildSpec("worker", func(sup erl.PID) (erl.PID, error) {
                return genserver.StartLink[WorkerState](sup, Worker{}, nil)
            }),
        }

        // Configure supervisor
        supFlags := supervisor.NewSupFlags(
            supervisor.SetStrategy(supervisor.RestForOne),
            supervisor.SetIntensity(5),
            supervisor.SetPeriod(60),
        )

        // Start supervisor
        supPID, err := supervisor.StartDefaultLink(self, children, supFlags)
        if err != nil {
            return err
        }

        // Keep running...
        for range inbox {
        }
        return nil
    })
}
```

### Dynamic Children via Callback

Use `StartLink` with a custom callback when children depend on runtime configuration:

```go
type MySupervisor struct{}

func (s MySupervisor) Init(self erl.PID, args any) supervisor.InitResult {
    config := args.(AppConfig)

    // Create children based on configuration
    children := make([]supervisor.ChildSpec, 0)

    // Always have a connection manager
    children = append(children, supervisor.NewChildSpec("conn_manager",
        func(sup erl.PID) (erl.PID, error) {
            return genserver.StartLink[ConnState](sup, ConnManager{}, config.DBUrl)
        },
    ))

    // Add workers based on config
    for i := 0; i < config.WorkerCount; i++ {
        id := fmt.Sprintf("worker_%d", i)
        children = append(children, supervisor.NewChildSpec(id, workerStartFn,
            supervisor.SetRestart(supervisor.Transient),
        ))
    }

    return supervisor.InitResult{
        SupFlags: supervisor.NewSupFlags(
            supervisor.SetStrategy(supervisor.RestForOne),
        ),
        ChildSpecs: children,
    }
}

// Start the supervisor
supPID, err := supervisor.StartLink(self, MySupervisor{}, appConfig)
```

### Nested Supervisors (Supervision Trees)

Create hierarchical fault isolation by nesting supervisors:

```go
children := []supervisor.ChildSpec{
    // Nested supervisor for database-related processes
    supervisor.NewChildSpec("db_supervisor",
        func(sup erl.PID) (erl.PID, error) {
            dbChildren := []supervisor.ChildSpec{
                supervisor.NewChildSpec("db_pool", dbPoolStart),
                supervisor.NewChildSpec("db_cache", dbCacheStart),
            }
            return supervisor.StartDefaultLink(sup, dbChildren,
                supervisor.NewSupFlags(supervisor.SetStrategy(supervisor.OneForAll)),
            )
        },
        supervisor.SetChildType(supervisor.SupervisorChild),
        supervisor.SetShutdown(supervisor.ShutdownOpt{Infinity: true}),
    ),

    // Nested supervisor for API handlers
    supervisor.NewChildSpec("api_supervisor",
        func(sup erl.PID) (erl.PID, error) {
            apiChildren := []supervisor.ChildSpec{
                supervisor.NewChildSpec("http_server", httpStart),
                supervisor.NewChildSpec("grpc_server", grpcStart),
            }
            return supervisor.StartDefaultLink(sup, apiChildren,
                supervisor.NewSupFlags(supervisor.SetStrategy(supervisor.OneForOne)),
            )
        },
        supervisor.SetChildType(supervisor.SupervisorChild),
        supervisor.SetShutdown(supervisor.ShutdownOpt{Infinity: true}),
    ),
}

// Top-level supervisor
supFlags := supervisor.NewSupFlags(
    supervisor.SetStrategy(supervisor.OneForOne),
    supervisor.SetIntensity(2),
    supervisor.SetPeriod(30),
)

supPID, err := supervisor.StartDefaultLink(self, children, supFlags)
```

### Named Supervisor

Register a supervisor under a name for lookup:

```go
supPID, err := supervisor.StartDefaultLink(self, children, supFlags,
    supervisor.SetName("my_supervisor"),
)

// Later, look up by name
pid := erl.WhereIs("my_supervisor")
```

## Public API Reference

### Starting Supervisors

#### `StartDefaultLink(self erl.PID, children []ChildSpec, supFlags SupFlagsS, optFuns ...LinkOpts) (erl.PID, error)`

Starts a supervisor with a static list of children. Simplest way to create a supervisor.

#### `StartLink(self erl.PID, callback Supervisor, args any, optFuns ...LinkOpts) (erl.PID, error)`

Starts a supervisor with a custom callback module for dynamic child configuration.

### Creating Child Specs

#### `NewChildSpec(id string, start StartFunSpec, opts ...ChildSpecOpt) ChildSpec`

Creates a child specification with functional options.

**Options:**
- `SetRestart(restart Restart)` - Set restart type (default: `Permanent`)
- `SetShutdown(shutdown ShutdownOpt)` - Set shutdown behavior (default: 5000ms timeout)
- `SetChildType(t ChildType)` - Set child type (default: `WorkerChild`)

### Configuring Supervisor Flags

#### `NewSupFlags(flags ...SupFlag) SupFlagsS`

Creates supervisor flags with functional options.

**Options:**
- `SetStrategy(strategy Strategy)` - Set restart strategy (default: `OneForOne`)
- `SetPeriod(period int)` - Set restart period in seconds (default: 5)
- `SetIntensity(intensity int)` - Set max restarts in period (default: 1)

### Link Options

#### `SetName(name erl.Name) LinkOpts`

Registers the supervisor under a name for lookup via `erl.WhereIs()`.

## How It Works

### Internal Implementation

The supervisor is implemented as a `genserver.GenServer` that:

1. **Sets TrapExit** to receive exit signals as messages instead of propagating them
2. **Starts children** sequentially in the order specified
3. **Receives `erl.ExitMsg`** when any linked child terminates
4. **Decides restart action** based on child's restart type and exit reason
5. **Tracks restart timestamps** in a rolling window for intensity checking
6. **Terminates children** in reverse start order during shutdown

### Child Termination Process

When terminating a child, the supervisor:

1. Spawns a `childKiller` helper process
2. Sends `exitreason.SupervisorShutdown` to the child
3. Waits according to `ShutdownOpt`:
   - **Timeout**: Wait up to N ms, then send `exitreason.Kill`
   - **BrutalKill**: Immediately send `exitreason.Kill`
   - **Infinity**: Wait forever
4. Monitors for `DownMsg` to confirm termination

### Exit Signal Flow

```
Child terminates
       ↓
Supervisor receives erl.ExitMsg (because TrapExit=true)
       ↓
Supervisor looks up ChildSpec by PID
       ↓
Check Restart type vs exit reason
       ↓
If should restart:
  - Increment restart counter
  - Check intensity (terminate self if exceeded)
  - Execute strategy (OneForOne/OneForAll/RestForOne)
       ↓
If should not restart:
  - Remove child from list (Temporary/Transient with clean exit)
```

## Differences from Erlang/OTP

This implementation follows Erlang supervisor semantics closely, with these notes:

| Feature | Erlang/OTP | erl-go |
|---------|-----------|--------|
| `simple_one_for_one` strategy | Supported | Not implemented (use regular supervisor) |
| `start_child/2` | Dynamic child addition | Not yet implemented |
| `terminate_child/2` | Dynamic child removal | Not yet implemented |
| `restart_child/2` | Manual restart trigger | Not yet implemented |
| `count_children/1` | Introspection | Not yet implemented |
| `which_children/1` | List children | Not yet implemented |
| Shutdown signal | `shutdown` atom | `exitreason.SupervisorShutdown` |
| Kill signal | `kill` atom | `exitreason.Kill` |

## Best Practices

1. **Always link children to the supervisor** in the start function using `genserver.StartLink`, `erl.SpawnLink`, etc.

2. **Use `Infinity` shutdown for supervisor children** to allow their subtrees to shut down gracefully.

3. **Choose the right restart type:**
   - `Permanent` for services that must always run
   - `Transient` for tasks that should succeed eventually
   - `Temporary` for fire-and-forget operations

4. **Set appropriate intensity limits** based on your failure expectations. Too low causes premature supervisor death; too high allows runaway restart loops.

5. **Structure supervision trees** to isolate failures. Group related processes under the same supervisor so failures in one area don't affect unrelated processes.

6. **Use `RestForOne`** when you have sequential dependencies (later children depend on earlier ones).

7. **Use `OneForAll`** sparingly, only when all children truly depend on each other.

## See Also

- [Erlang Supervisor Concepts](https://www.erlang.org/doc/system/sup_princ.html)
- [Erlang Supervisor Module](https://www.erlang.org/doc/apps/stdlib/supervisor)
- [Introduction to erl-go](./intro.md)

