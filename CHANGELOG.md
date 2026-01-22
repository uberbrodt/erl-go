# Changelog

## [Unreleased]

### Changed
- Rewrote `docs/supervisor.md` with comprehensive coverage of supervisor concepts,
  restart strategies, restart types, shutdown options, and usage examples including
  static children, dynamic children via callback, nested supervision trees, and
  named supervisors.
- Added extensive Go doc comments throughout the supervisor package:
  - `api.go`: Expanded documentation for `StartDefaultLink`, `StartLink`, `SetName`,
    and `LinkOpts` with detailed parameter descriptions, return values, and examples
  - `child_spec.go`: Documented `ChildSpec` fields, `StartFunSpec` contract,
    `NewChildSpec`, and all functional options (`SetRestart`, `SetShutdown`, `SetChildType`)
  - `child_killer.go`: Added internal documentation for the child termination helper
  - `supervisor.go`: Documented `SupFlagsS`, `SupervisorS`, `InitResult`, `Supervisor`
    interface, and all GenServer callback implementations with algorithm descriptions
  - `supervisor_state.go`: Documented internal state management and restart intensity algorithm
  - `types.go`: Expanded documentation for `Strategy`, `Restart`, `ShutdownOpt`, and
    `ChildType` constants with usage guidance and example scenarios
  - `doc.go`: Enhanced package-level documentation with quick start example and
    comprehensive overview of all supervisor features
- Clarified documentation for `Terminate` callback in `genserver.GenServer`
  interface and `gensrv.RegisterTerminate` function to accurately describe
  when the terminate handler is invoked:
  - On `Stop()` calls
  - On callback panics or errors
  - On exit signals when trapping exits (via `erl.ProcessFlag` with `erl.TrapExit`)
- Updated docs to note that exit signals cause immediate termination WITHOUT
  calling Terminate when the GenServer is not trapping exits.

### Added
- Implemented a dynamic child management API for `supervisor`, bringing it
  closer to feature parity with Erlang/OTP. This includes `StartChild`,
  `TerminateChild`, `RestartChild`, `DeleteChild`, `WhichChildren`, and
  `CountChildren`.
- Add `InitOKTrapExit` helper function to `testserver` package for initializing
  test servers that trap exit signals.
- Add `SetTerminate` method to `testserver.Config` for registering Terminate
  handlers in test GenServers.
- Add `AddContinueHandler` method to `testserver.Config` for registering
  HandleContinue handlers in test GenServers.
- Add `bin/test-loop.sh` script for continuous test execution with logging.
- Add comprehensive documentation to the supervisor package including
- Add comprehensive API documentation to genserver and gensrv packages:
  - Document all `GenServer` interface methods (`Init`, `HandleCall`, `HandleCast`,
    `HandleInfo`, `HandleContinue`, `Terminate`) with usage guidance
  - Document result types (`InitResult`, `CallResult`, `CastResult`, `InfoResult`)
    with field descriptions
  - Expand gensrv `doc.go` with complete usage examples covering continuations,
    panic recovery, error handling, and the `From` parameter for deferred replies
  - Add detailed documentation to all gensrv `Register*` functions, `Start*`
    functions, and public types (`CastHandle`, `CallHandle`, `GenSrvOpt`)

### Changed
- Refactored `application` package to use `gensrv` instead of manual Runnable
  implementation. The application GenServer now uses proper message handlers
  (`RegisterInit`, `RegisterInfo`, `RegisterTerminate`) for cleaner separation
  of concerns and proper lifecycle management. The `App.Stopped()` method now
  also checks if the underlying process is alive.
- Improved test reliability in `TestRegisteredCount_ReturnsCorrectCount` by using
  relative count comparisons and adding proper cleanup.
- Added tests for panic recovery in genserver callbacks.
- Refactored panic recovery logic into shared `panicToException` helper function.
- Updated docs

### Fixed
- Fixed TOCTOU race condition in `Register` where a process could exit between
  the `IsAlive` check and `setName` call, leaving stale entries in the registry.
  Registration now atomically verifies process status and sets the name.
- Fixed `genserver.Stop` not invoking the `Terminate` callback. The stop
  mechanism now sends an internal `stopRequest` message that triggers the
  `Terminate` callback before exiting, matching Erlang GenServer semantics.
  `exitreason.Kill` still bypasses `Terminate` per Erlang behavior.
- Fixed `Terminate` callback not being called when `HandleCall`, `HandleCast`,
  `HandleInfo`, or `HandleContinue` panics. Panic recovery now invokes
  `Terminate` before the process exits, matching Erlang GenServer semantics.



## 0.19.1 2025-3-11

### Added
- Added documentation for the `supervisor` package.
- Added a `WaitOn` method to `x/erltest/testcase` that will add a dependency
whose `Wait()` method will be called in parallel with all other "Waitables" when
`testcase.Case.Assert()` is called.
- Added `ParseOpts` to `erl/erltest/expect` to help transition users to
`x/erltest`

### Changed
- Return the exitreason from GenCaller, not the whole exitmsg.
- Make `erl/erltest/expect.ExpectOpts` public, along with some methods to get
  Times and ExType. This is to enable consumers to transition to `x/erltest`
  without fully re-writing their tests.


## 0.19.0 2025-3-5

### Changed
- added `Drain` method to inbox
- `erl/erltest` is now deprecated. The matchers and logging were not great, and
  there were some race conditions that would be difficult to resolve.
- `make test` will now use `-shuffle` and `-failfast` testflags.
- Use new `x/erltest` everywhere.


### Fixed
- Fixed message dropping in `erl/internal/inbox`.
- The goroutine spawned by `Inbox.Channel()` now terminates when inbox is closed.
- Processes will drain the inbox when exiting and process link and monitor
  signals. There was an edge case where process Runnable could fail before the
  link/monitor signals were processed.
- Starting a genserver is now synchronous for Supervisors or any process that is
  trapping exits, and the calling process will not need to handle the return
  error and the exit signal.

### Added
- `erl/x/erltest` is the replacement for `erl/erltest` that uses `gomock` Matchers and more
  closely simulates its behaviour. Eventually it will be moved to the
  `erl/erltest` package before 1.0.
- Added  test stub, `testserver.TestServer` which can be used to create a
  GenServer for test purposes without defining a new state. Also includes some
  helper Init Functions.

### Changed
- removed dependence on `exp` package. Only major changes was handling `iter.Seq` when using
  the `maps` package.
- upgraded deps
- added a go.mod to `examples/basic`, making it a module.
- return down signal in erl.Monitor if linkee is already down


## 0.18.1 2024-12-13

### Changed

The `erltest.TestReceiver` Expect* methods will now add an expectation for each
matchTerm, instead of overwriting the last one. This makes the expectation
setting more similar to gomock, but with a caveats: If the first expectation
is an absolute matcher like `expect.Times()`, then that expectation will match
until the `Wait()` is done.


### Added

ExpectCallReply() was added to the `erltest.TestReceiver`. The big improvement
over `ExpectCall()` is that you can specify a value to reply with when a match
is executed. Before, all Call Expectations needed to be functions so that the
replies could be sent.



### Fixed

- `erltest.TestReceiver` was not always failing when one of the expecations
failed. Unfortunately we can't indicate which exception failed until we do a
refactor, but at least we'll fail now.


## 0.18.0 2024-12-01

A fairly large release, focused on performance and improving the testing
experience


### Added

- inbox.Inbox struct for replacing the buffered channels in `erl.process`.
- `erltest.TestReceiver.StartSupervised` will run a start link function with the
  TestReceiver PID and keep track of the created PID for cleanup in the `Stop()`
  function.
- Added an `exitwaiter`, which accepts a `sync.WaitGroup` and a pid to wait on.
  It will call `Done()` on the waitgroup whenever it receives the DownSignal
  for the `pid`.

### Changed

- erltest.WaitTimeout does not stop the testreceiver. It now just indicates how
  long until the assertions `Times`, `Absolute` and `AtMost` are considered
  "fulfilled". So if you have an expectation of receiving a message
  `expect.Times(3)`, if you receiver a 4th messages AFTER the WaitTimeout, your
  test will not fail. This is a compromise for a hard problem to solve, given
  that test runtimes vary widely between developer computers and CI.
- erltest.TestReceiver has an "adaptive" timeout for test exit now. If the "-timeout" value is  less than the DefaultReceiverTimeout, it will be used
  instead. The goal is always exit before the timeout so that we print the
  missed expectations.
- Don't use the `t.Log` calls inside the TestReceiver process; it's very
  frustrating for a test to fail because it logged some meaningless information
  after the test ended.



### Fixed

- erltest.TestReceiver.Stop() is now synchronous and returns after all linked
processes and the test receiver returns.
- processes that were being monitored were not getting demonitor signals from
  their monitors; this meant that references to dead processes would accumulate
  in the `process.monitors`, which caused big memory leaks for applications
  using `erl.SendAfter`.






## [0.16.1] 2024-07-31


This moves [gensrv] and the [erltest] pkgs into a stable release. We're still
not to a 1.x release yet, but these interfaces should be pretty stable.

### Changed
- `erl.Exit` no longer panics if the self or target are not running.

## [0.16.0-rc.8] 2024-06-06

### Changed
- make gensrv handlers cast free.

### Changed
- cleanup expectation to use [Fail] method
- rename expect.NewExpectation to expect.New
- add And() method and check children expectations in Satisifed()
- Change ExpectationFailure.MatchType to Match
- renamed [expect.Simple] to [expect.Expect]



## [0.16.0-rc.2] 2024-02-21

### Changed
- cleanup expectation to use [Fail] method
- rename expect.NewExpectation to expect.New
- add And() method and check children expectations in Satisifed()
- Change ExpectationFailure.MatchType to Match
- renamed [expect.Simple] to [expect.Expect]


##[0.16.0-rc.1] 2024-02-21

### Changed
- Refactored expects pkg out of erltest
- A system is now in place to use "nested expectations" by means of
  "attaching" to a test receiver

##[0.16.0-rc.0] 2024-02-19
Updates to `erltest`

### Added
- `Never()` expectation option.
- `erltest.Expectation`, which exposed the `Check()` and `Satisified()` methods.
  This will allow for wrapping of `TestReceiver` and adding sub-checks of
  matching messages.

### Changed
- Added `erltest.ReceiverOpt` to configure timeouts
- `erltest.AtMost` will cause `TestReceiver.Wait()` to always wait until
  WaitTimeout is exceeded.


### Removed
- `TestReceiver.WaitFor()` was replaced by the `WaitTimeout` option to the
  receiver.

## [0.15.0] 2024-02-18
### Added
#### erltest
- `ExpectCast` and `ExpectCall` methods for better mocking of genserver Cast and
  Call messages.
- Added a `Stop` method to `TestReceiver`. Useful to test Exit/Down handling for
  processes.

### Fixed
- don't log exit msgs at all in the test receivers. Related to very occasional
  panics caused by logging in test receiver after the test goroutine has ended.

## [0.14.1] 2024-02-18
### Fixed
- bug in `erltest.Times` where numbers greater than 1 would be marked as
  failures, even if the exact count was reached.

## [0.14.0] 2024-02-18

### Added
 - `check` package that contains assertions that do not call `t.Test.FailNow()`,
   which must not be called outside of a test goroutine.

### Fixed
- `TestReceiver` killed if a `TestExpectation` panicked. Fixed.


## [0.12.0] 2024-02-17

### Changed
- marked [erl.TestReceiver] as deprecated

### Added
- added the [erltest] package, which contains a new TestReceiver implementation
  that allows for setting Expectations on messages that it will receive. The
  following types of expectations can be set:
   - Exactly N Times
   - At Most N Times
   - Any (or zero) Times
   - At Least N Time
   - Received As Nth message (Absolute)

## [0.11.0] 2024-02-16

### Changed
- Made `genserver` `CastRequest`, `CallRequest`, and `CallReply` are public
  types, and their properties have been made public as well. While I dislike a breaking
  change that exposes some of the inside baseball, these structs can show up in
  some situations for the enduser, if nothing else in `erl.TestReceiver` if you
  have it stand-in for a GenServer.


## [0.10.0] 2024-02-08

### Added
- exitreason.TestExit so testreceiver knows to shutdown

### Changed
- Removed errant logging statement
- Refactored Process to move name unregistering to the `exit` method

### Fixed
- `genserver.Cast` always returned an error! It should only return an error
  if an `erl.Name` is used and it is not registered.

## [0.9.1] 2024-01-29

### Added
 - SetName and SetStartTimeout options to `gensrv` pkg.


## [0.9.0] 2024-01-29

### Added
- `gensrv` package, a wrapper around the `genserver` package

## [0.8.2] 2023-12-15

### Removed

- print statements in one of the port split functions

## [0.8.1] 2023-12-11

### Changed

- `erl.SendAfter` returns a `erl.TimerRef`. This is the argument to
  `erl.CancelTimer`

### Added

- `erl.CancelTimer` will cancel a timer if it exists. If the `TimerRef` is
  invalid, an error will be returned.

## [0.8.0] 2023-12-11

### Fixed

- flaky `application` test was not waiting for application ExitMsg before verifying
  the PID status.

### Changed

- [port] is largely re-written. The major change is that non-error exits from
  the Port will return `exitreason.Normal`, which will only appear to the
  PortOwner if it is trapping exits.
- Besides that, many options were added to `port`. It is considered largely
  complete at this point.

- In `genserver` we have a small but significant change to the `HandleContinue`
  return type. You can now return a third return item to Continue to another
  handler.

## [0.7.0] 2023-11-26

### Fixed

- fixed race condition around the trapExit flag
- fixed arg not passed to App struct (and then the root Supervisor)
- made a genserver.DefaultOpts() function.
- removed App.Self(); wasn't necessary and introduced potential issues.
- allow RootPID to receive an exitSignal from `erl.Exit`
- handle panics in GenServer.Init so that an error is returned instead of
  a timeout.

### Added

- tests for application, supervisor

### Removed

- application.App.Self() removed. Not really a use case for it and issues could
  arise from linking processes to it.

### Changed

- refactor in recurring task to remove duplication

## [0.6.1] 2023-11-21

### Fixed

- The application would hang if the root supervisor failed to start. Someone
  should really write some tests for that package.

## [0.6.0] 2023-11-21

### Changed

- Changed the way we count test coverage to remove the testing helpers.

### Added

- `recurringtask` package supports starting a process that will execute every
  `Interval`. This is a common use case for GenServers, and cuts down on the
  amount of boilerplate needed.

- `genserver.InheritOpts` allows passing in a struct that matches an interface,
  which makes building abstractions on top `GenServer` easier.

## [0.5.1] 2023-11-20

### Fixed

- Fix a bug with Supervisor where the ChildKiller process would get stuck in a
  loop because it did not handle the inbox closure.

- Application was not shutting down it's supervision tree when Stop() was called.
  We now wait 60s for this to happen before returning.

- Port was sending exitreason.Normal if the external process exited, which will
  not send an ExitMsg to linked processes that are not trapping exits. Changed
  to Shutdown and added ShutdownReason constants

## [0.5.0] 2023-11-17

### Added

- GenServers - Complete

- Applications - The root node of an application, light wrapper around a
  supervisor. Takes a `context.CancelFunc`, the intention is to accept a
  SignalHandler so that it will shutdown if killed by the OS or systemd, etc.

- Supervisors - Mostly complete. Missing some methods to dynamically
  modify a static Supervisor and DynamicSupervisors.

- Process Registration - Added a single "builtin" registry. There is a plan to add a
  Registry process so that we can scale out the number of named
  processes if needed.

- Ports [EXPERIMENTAL] - These are processes that handle external I/O
  with child processes started with `os.exec.Cmd`. In it's current
  state, it is not supervisable, so the recommended usage is to start it
  from a GenServer in a supervision tree. Calling `port.Open` will
  establish a Link so if the port exits the server will crash and
  restart it.

- Tasks [EXPERIMENTAL] - Go routines are great and super easy to use,
  but sometimes we have long-running ones that we want to keep tabs on.
  To fill that gap, Tasks accept a taskFun and a cleanupFun so you get
  the ease of a Goroutine but you can also plug it into a Supervisor
  tree and take advantage of restarts and ordered shutdown.

- Added an exitreason package to try and handle the myriad reasons a
  process can exit in a Go-like manner.

- Added a RootNode(). It's basically a process that logs messages, but
  it makes ExitMsgs make more sense and is roughly equivalent to
  ActorSystems

- Added Exit method to erl.

### Changed

- made process.id an atomically incremented int. Gurantees uniqueness
  and is easier to read in the logs. This shouldn't affect users of the pkg.











