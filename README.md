# erl-go
[![GoDoc](https://pkg.go.dev/github.com/uberbrodt/erl-go?status.svg)](https://pkg.go.dev/github.com/uberbrodt/erl-go)


An Erlang like Actor System for Go

## TODO

- [x] gen_server
  - [x] functions
    - [x] call
    - [x] cast
    - [x] reply
    - [x] stop
    - [x] start
    - [x] start_link
    - [x] start_monitor
  - [x] interface
    - [x] HandleCall
    - [x] HandleCast
    - [x] HandleInfo
    - [x] HandleContinue
    - [x] Init
    - [x] Terminate
- [x] ports (to support external commands)
  - [x] Open
  - [x] Close
- [ ] supervisor interface

  - [x] StartLink
  - [ ] StartChild
  - [ ] RestartChild
  - [ ] TerminateChild
  - [ ] DeleteChild
  - [ ] CountChildren
  - [ ] WhichChildren

- [x] restarts

  - [x] permanent
  - [x] transient
  - [x] temporary

- [ ] strategies
  - [x] one-for-one strategy
  - [x] one-for-all strategy
  - [x] rest-for-one strategy
  - [ ] simple-one-for-one strategy (might do an Elixir and make a
        `DynamicSupervisor`)
  - [x] Restart intensity exits
- [x] Application: a root for a supervision tree
