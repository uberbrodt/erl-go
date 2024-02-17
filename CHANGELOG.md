# Changelog


##[0.11.0] 2024-02-16

### Changed
- Made `genserver` `CastRequest`, `CallRequest`, and `CallReply` are public
  types, and their properties have been made public as well. While I dislike a breaking
  change that exposes some of the inside baseball, these structs can show up in
  some situations for the enduser, if nothing else in `erl.TestReceiver` if you
  have it stand-in for a GenServer.


##[0.10.0] 2024-02-08

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
