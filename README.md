# erl-go
[![GoDoc](https://img.shields.io/badge/godoc-reference-blue.svg)](https://pkg.go.dev/github.com/uberbrodt/erl-go)


An Erlang like Actor System for Go, based on message passing and process
supervision as pioneered in the [Open Telecom Platform](https://en.wikipedia.org/wiki/Open_Telecom_Platform) at Ericsson.

## Why?

Go provides many standard concurrency primitives, but the two most unique ones are
Channels and Goroutines, which allow for a lightweight style of communication
between multiple processes in contrast to traditional mutexes.

Here's a simple example of using those primitives:

```go
package main
import "fmt"

func main() {
    inbox := make(chan any)

    fmt.Println("Starting Goroutine")
    go func() {
        for  {
            msg := <- inbox
            fmt.Printf("Goroutine got msg: %+v\n", msg)

        }
    }()

    inbox <- "Msg 1"
    inbox <- "Msg 2"
    inbox <- "Msg 3"
}
```

One way communication between the main thread and a single go routine are pretty
well covered. Let's try a more complex example. The following code is a
simulation of a pretty common pattern:

1. Process2 receives messages from main.
2. Process2 does something and forwards to Process1 for further processing, with
   an expected acknowledgement (presumed that we would have retry code, not
   included here).
3. Process1 then acknowledges the msg and also sends an ack to the main thread.
4. Process2 sends it's acks from process 1 over to the main thread.

```go
package main
import "fmt"

func main() {
    func1In := make(chan any)
    func1Out := make(chan any)
    func2In := make(chan any)
    func2Out := make(chan any)

    fmt.Println("Starting Goroutine")
    go func() {
        for msg := range func1In {
            fmt.Printf("Goroutine1 got msg: %+v\n", msg)
            func2In <- fmt.Sprintf("Goroutein1 Ack msg: %+v\n", msg)
            func1Out <- fmt.Sprintf("Goroutein1 Ack msg: %+v\n", msg)


        }
    }()

    go func() {
        for msg := range func2In {
            fmt.Printf("Goroutine2 got msg: %+v\n", msg)
            func1In <- fmt.Sprintf("Proxied msg: %+v\n", msg)
            func2Out <- fmt.Sprintf("GoRoutine2 Ack Msg: %+v\n", msg)

        }
    }()

    func2In <- "Msg 1"
    func2In <- "Msg 2"
    func2In <- "Msg 3"


    for out := range func2Out {
        fmt.Printf("Goroutine2 out: %+v\n", out)
    }

    for out := range func1Out {
        fmt.Printf("Goroutine2 out: %+v\n", out)
    }

}
```

However if you run the above it will deadlock:
```
Starting Goroutine
Goroutine2 got msg: Msg 1
Goroutine1 got msg: Proxied msg: Msg 1

fatal error: all goroutines are asleep - deadlock!

goroutine 1 [chan send]:
main.main()
    /tmp/sandbox4198745348/prog.go:31 +0x1d5
```


There's a few approaches to get out of this:

1. Buffered channels, but if the setting is wrong it will still deadlock.
2. Restructure to remove goroutine communication, but this may actually be
   structually important for long running goroutines.


`erl-go` takes the position that long running goroutines can communicate safely
between each other by providing abstractions such as a Process ID (PID) and
`erl.Send` to prevent the common deadlock scenarios. Additionally, abstractions
like `supervisor` and `gensrv` handle the complex panic and error handling to
ensure that processes stay alive and reliably notify the system when they fail.




## Compared to Erlang/Elixir

The following is a punch list of features from the Elixir/Erlang/OTP ecosystem
that erl-go plans to support:

- [x] gen_server [x] functions
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
- [X] supervisor interface

  - [x] StartLink
  - [X] StartChild
  - [X] RestartChild
  - [X] TerminateChild
  - [X] DeleteChild
  - [X] CountChildren
  - [X] WhichChildren

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
- [ ] Additional Behaviours:
    - [ ] Finite State Machines

The following features are NOT planned at this time, though that may change in
the future:

- Distributed Erlang
- OTP Releases
- [Priority Messages](https://www.erlang.org/blog/highlights-otp-28/#priority-messages)
- [Significant Processes and Automatic Supervisor Shutdown](https://www.erlang.org/doc/apps/stdlib/supervisor#auto_shutdown)

Tracing and diagnostic improvements are planned, but it's not clear what shape
they will take yet.
