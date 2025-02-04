# Erl-Go: A Supervised Actor Framework for Go.

This framework is designed with the Erlang/OTP (and by extension Elixir)
programming languages, though knowledge of either is not necessary to understand
and utilize erl-go. It is recommended that you thoroughly understand the following
Go features before you read this document, as we'll use them as comparison
against Golang implementations. The rest of this document introduces the
basic concepts of the erl-go system and how they fit together.

## Processes and Message Passing

The building block of erl-go is the _process_. A process is a wrapper around a
Go routine with the following properties that is started by calling
[erl.Spawn](https://pkg.go.dev/github.com/uberbrodt/erl-go@v0.15.0/erl#Spawn),
[erl.SpawnLink](https://pkg.go.dev/github.com/uberbrodt/erl-go@v0.15.0/erl#SpawnLink),
and [erl.SpawnMonitor](https://pkg.go.dev/github.com/uberbrodt/erl-go@v0.15.0/erl#SpawnMonitor).
The main distinguishing features of a process as compared to a Go routine are
that it can be referred to by a reference called a [Process Identifier (PID
   for short)](https://pkg.go.dev/github.com/uberbrodt/erl-go@v0.15.0/erl#PID)
and it has the ability to send and receive _signals_.

### PID
A PID reference is used to send signals between processes (more on this
shortly) and has a status that can be safely checked even after the goroutine
has ended.

### Signals

_Signals_ are the asynchronous communication mechanism of processes. There are
various kinds of signals, but the main ones we are concerned with will be:

* **Link/Unlink**
* **Monitor/Demonitor**
* **Message**
* **Exit**
* **Down**

Let's look at an example of sending an Exit signal to a process:

```go
package main

import (
	"log"
	"time"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
)

type P struct{}

func (p P) Receive(self erl.PID, inbox <-chan any) error {
	for {
		select {
		case _, ok := <-inbox:
			if !ok {
				return nil
			}
		case <-time.After(3 * time.Second):
			log.Printf("The time is: %s\n", time.Now().String())
		}
	}
}

func main() {
	pid := erl.Spawn(P{})
	time.Sleep(10 * time.Second)
	log.Printf("Process is alive?: %t", erl.IsAlive(pid))
	erl.Exit(erl.RootPID(), pid, exitreason.Kill)
	time.Sleep(3 * time.Second)
	log.Printf("Process is alive?: %t", erl.IsAlive(pid))
}
```

The output of the above program should look something like this:

```
-> % go run ./docs/intro/main.go
2024/04/30 23:43:33 The time is: 2024-04-30 23:43:33.575192198 -0500 CDT m=+0.000423650
2024/04/30 23:43:36 The time is: 2024-04-30 23:43:36.577607096 -0500 CDT m=+3.002838658
2024/04/30 23:43:39 The time is: 2024-04-30 23:43:39.579781589 -0500 CDT m=+6.005013031
2024/04/30 23:43:42 The time is: 2024-04-30 23:43:42.582135672 -0500 CDT m=+9.007367344
2024/04/30 23:43:43 Process is alive?: true
# no more timestamps printed because process is dead
2024/04/30 23:43:46 Process is alive?: false

```

First we declare an [erl.Runnable](https://pkg.go.dev/github.com/uberbrodt/erl-go@v0.15.0/erl#Runnable) which is an interface with a single function, `Receive` which accepts the PID of the current process and a channel for incoming messages. When it is passed to `erl.Spawn`, a Goroutine is started to receive signals from other processes and another is started to to send Message signals from the first goroutine to the Runnable via the `inbox`.

All well-behaving processes are expected to consume the inbox until it is
closed. In Go there is no way to interrupt an executing Goroutine, so failure to
return when an inbox is closed will cause a process leak. Later we'll look at
higher level abstractions that handle this logic for you.


