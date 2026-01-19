# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

erl-go is an Erlang-like Actor System for Go. It implements core Erlang/OTP primitives including processes, monitors, links, supervisors, and gen_server behaviors, providing a foundation for building concurrent, fault-tolerant applications in Go.

The goal of this project is to be a useful concurrency framework for Go developers that solves the following problems that are found with just basic go routines:

1. Deadlock free bi-directional communication between go routines.
2. Supervision trees that ensure a go routine is restarted from an initial good
   state if a panic occurs or exits programatically.

## Non-Blocking/Deadlock Free Goroutine Communication


For unbuffered channels, any send operation is blocking to the caller. If we
want to achieve high availability via concurrency within a single Go process, we
need a way to make these communications non-blocking. We emulate the Erlang
system by having the Goroutines communicate via an intermediary process that
uses `inbox` package to store incoming messages and call these `Processes` (via
the `Process` interface).

While it's preferable to communicate asynchronously, a synchronous abstraction
is useful. This is where the `genserver.Call` primitive comes into play. By
using a configurable timeout (and gen_caller under the hood), we can make a
synchrous call that will be non-blocking.


## Development Commands

### Testing
```bash
# Run all tests (uses gotestsum, shuffled, race detector)
make test

# Run specific test or pattern
make test TEST_RUN=TestName

# Run specific package tests
make test TEST_ARG=./erl/supervisor

# Run tests with different timeout (default 2m)
make test TIMEOUT=5m

# Run full test suite including slow/integration tests
make test-full

# Watch mode - rerun tests on file changes
make test-watch
```

### Building and Formatting
```bash
# Build all packages
make build

# Format code (uses gofumpt)
make format

# Run static checks (formatting + build tags)
make check
```

### Coverage
```bash
# Generate coverage report and open in browser
make coverage-report
```

### Documentation
```bash
# Start local documentation server at http://localhost:8081
make view-docs
```

## Tests

`gotest.tools/v3/assert` is the standard assertions framework.

E2E/Integration tests that verify call and cast expectations via a TestReceiver
should use the `erl/x/erltest` package.


```go
//go:build integration
```

This is enforced by `./scripts/check-test-build-tags` in CI.

## Architecture

### Core Concepts

**Process System**: The `erl` package provides the foundational process system. Processes wrap goroutines and communicate via channels, providing Erlang-style message passing with monitors, links, and exit signals.

**Key Primitives** (erl/erl.go):
- `Spawn(r Runnable) PID` - Create a new process
- `SpawnLink(self PID, r Runnable) PID` - Spawn and link processes (exit signals propagate)
- `SpawnMonitor(self PID, r Runnable) (PID, Ref)` - Spawn with one-way monitoring
- `Monitor(self PID, pid PID) Ref` - Monitor another process for exit
- `Link(self PID, pid PID)` - Bi-directional exit propagation
- `Send(pid PID, term any)` - Asynchronous message send
- `ProcessFlag(self PID, flag ProcFlag, value any)` - Set process flags (e.g., TrapExit)

**Exit Signals**: When a process exits, linked processes receive exit signals. Use `ProcessFlag(self, erl.TrapExit, true)` to convert exit signals into `ExitMsg` that can be handled in the process receive loop.

### GenServer

Two implementations exist:

1. **genserver** (erl/genserver/) - Lower-level, requires implementing the `GenServer[State]` interface with all callbacks (Init, HandleCall, HandleCast, HandleInfo, HandleContinue, Terminate)

2. **gensrv** (erl/gensrv/) - Higher-level, registration-based API. Register handlers for specific message types using functional options:
   - `RegisterInit[State, Arg](func)` - Initialize state
   - `RegisterCall[State, Msg](matchType, handler)` - Synchronous requests
   - `RegisterCast[State, Msg](matchType, handler)` - Asynchronous messages
   - `RegisterInfo[State, Msg](matchType, handler)` - System/info messages
   - `RegisterContinue[State, Msg](matchType, handler)` - Deferred processing
   - `RegisterTerminate[State](handler)` - Cleanup on exit

Use `gensrv` for simpler servers with message-specific handlers. Use `genserver` when you need full control over the callback interface.

### Supervisor

Located in `erl/supervisor/`. Implements OTP supervisor patterns for fault tolerance:

**Restart Strategies**:
- `OneForOne` - Only restart the failed child
- `OneForAll` - Restart all children when one fails
- `RestForOne` - Restart failed child and all children started after it

**Restart Types** (per child):
- `Permanent` - Always restart
- `Transient` - Restart only on abnormal exit (not Normal/Shutdown)
- `Temporary` - Never restart, remove from supervisor

**Configuration**:
```go
supervisor.NewSupFlags(
    supervisor.SetStrategy(supervisor.OneForOne),
    supervisor.SetIntensity(3),  // max restarts
    supervisor.SetPeriod(5),      // within N seconds
)
```

Supervisors implement `genserver.GenServer` internally and trap exits to handle child failures.

### Application

Located in `erl/application/`. Provides a root for supervision trees with automatic shutdown on restart intensity exceeded.

Pattern (see application.go:8-56 for full example):
```go
type MyApp struct{}

func (ma *MyApp) Start(self erl.PID, args any) (erl.PID, error) {
    children := []supervisor.ChildSpec{ /* ... */ }
    return supervisor.StartDefaultLink(self, children, supervisor.NewSupFlags())
}

func (ma *MyApp) Stop() error {
    // cleanup
    return nil
}

func main() {
    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
    defer cancel()

    app := application.Start(&MyApp{}, config, cancel)
    <-ctx.Done()
    if !app.Stopped() {
        app.Stop()
    }
}
```

### Port

Located in `erl/port/`. Wraps external system processes (os/exec.Cmd) in the process system:
- Port owner receives stdout as `PortMessage` line-by-line (customizable with SplitFunc)
- Non-zero exit codes send `ExitMsg` to port owner
- Use `ReturnExitStatus` option to get `PortExited` msg regardless of exit code
- Ports are linked to their owner by default

### Testing Framework (erltest)

Located in `erl/x/erltest/`. Provides utilities for testing actor-based code:
- `TestReceiver` - Helper for receiving and asserting on messages in tests
- `Expect` - Message expectation system for async message verification
- Pattern matchers and assertion helpers

## Package Structure

- `erl/` - Core process primitives and signals
- `erl/genserver/` - GenServer behavior (low-level interface)
- `erl/gensrv/` - GenServer behavior (registration-based)
- `erl/supervisor/` - Supervisor behaviors and child specs
- `erl/application/` - Application root container
- `erl/port/` - External process integration
- `erl/exitreason/` - Exit reason types and predicates
- `erl/timeout/` - Timeout handling utilities
- `erl/x/erltest/` - Testing framework for actor code
- `chronos/` - Time utilities (e.g., `chronos.Dur("5s")`)

## Key Patterns

### Message Passing
Always use `erl.Send(pid, msg)` or `genserver.Cast(erl.Dest, msg)` for async communication. For synchronous calls, use `genserver.Call()`.

### Error Handling
Exit reasons follow Erlang conventions:
- `exitreason.Normal` - Clean shutdown
- `exitreason.Shutdown(err)` - Controlled shutdown with reason
- `exitreason.Exception(err)` - Unexpected error
- `exitreason.Ignore` - Special case to cancel process start

### Panic Recovery
**All panics in user code are automatically recovered at the Process level** (erl/process.go:93-106). When a panic occurs:

1. The panic is caught by the Process wrapper's defer/recover
2. Converted to `exitreason.Exception` with stack trace
3. Process exits cleanly (all links and monitors are notified)
4. Linked supervisor receives exit signal and can restart the process
5. Restarted process begins with fresh state from Init

**Key guarantees:**
- Panics in any GenServer callback (Init, HandleCall, HandleCast, HandleInfo, HandleContinue, Terminate) are caught
- `genserver.Call()` operations return error rather than hanging when server panics
- Process cleanup always happens (links/monitors notified) even if Terminate panics
- Supervision trees restart processes from clean state after panic

**You don't need to add panic recovery in your callback code.** The Process-level handler provides universal protection. Just implement your business logic and let the supervision tree handle failures.

### Supervision Trees
Build hierarchical fault-tolerance by nesting supervisors. Use Application as the root with signal handling to coordinate shutdown.

## Important Notes

- Go 1.23.0+ required
- Tests run with race detector enabled by default
- Coverage excludes test framework code via `./scripts/rm-test-fw-from-coverprofile`
- Must use GNU make (on macOS: `alias make=gmake`)
- Debug logging controlled via `erl.SetDebugLog(bool)`

### Erlang

Since the design of this project is inspired by erlang, it's important to
understand the basic design philosophy. Here's some helpful links to consult if
looking for inspiration for new features or to identify where we deviate from
erlang/elixir.

- [Processes](https://www.erlang.org/doc/system/ref_man_processes.html)
- [gen_server 1](https://www.erlang.org/doc/system/gen_server_concepts.html#synchronous-requests---call)
- [gen_server 2](https://www.erlang.org/doc/apps/stdlib/gen_server)
- [supervisor 1](https://www.erlang.org/doc/system/sup_princ.html)
- [supervisor 2](https://www.erlang.org/doc/apps/stdlib/supervisor)
- [Dynamic Supervisors](https://hexdocs.pm/elixir/dynamic-supervisor.html)
- [Erlang repo](https://github.com/erlang/otp)
-


