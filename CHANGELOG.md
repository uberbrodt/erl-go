# Changelog

## [Unreleased]

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