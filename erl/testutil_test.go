package erl

import (
	"sync"
	"testing"
	"time"

	"github.com/uberbrodt/erl-go/chronos"
	"github.com/uberbrodt/erl-go/erl/erltest/check"
	"github.com/uberbrodt/erl-go/erl/exitreason"
)

type TestRunnable struct {
	t        *testing.T
	expected string
}

type testRunnableSyncArg struct {
	wg     *sync.WaitGroup
	actual string
}

func (tr *TestRunnable) Receive(self PID, inbox <-chan any) error {
	// tr.t.Logf("TestRunnable waiting for incoming message")
	for {
		select {
		case m, ok := <-inbox:
			if !ok {
				return exitreason.Normal
			}
			switch msg := m.(type) {
			case testRunnableSyncArg:
				check.Equal(tr.t, msg.actual, tr.expected)
				msg.wg.Done()
				return exitreason.Normal

			default:
				tr.t.Logf("Got unhandled message in TestRunnable")
			}
		case <-time.After(chronos.Dur("5s")):
			return exitreason.Timeout
		}
	}
}

type TestSimpleRunnable struct {
	t    *testing.T
	exec func(self PID, done chan<- int, inbox <-chan any) error
	done chan<- int
}

func (tsr *TestSimpleRunnable) Receive(self PID, inbox <-chan any) error {
	return tsr.exec(self, tsr.done, inbox)
}

type TestSimpleServer struct {
	t       *testing.T
	timeout time.Duration
	exec    func(self PID, done chan<- int, msg any)
	done    chan<- int
}

func (tss *TestSimpleServer) Receive(self PID, inbox <-chan any) error {
	for {
		select {
		case m := <-inbox:
			tss.t.Logf("TestSimpleServer got message: %+v", m)
			tss.exec(self, tss.done, m)
		case <-time.After(tss.timeout):
			tss.t.Errorf("TestSimpleServer timed out after %s", tss.timeout)
			tss.done <- 1
		}
	}
}

type TestServer struct {
	t       *testing.T
	timeout time.Duration
	exec    func(ts *TestServer, self PID, inbox <-chan any) error
	done    chan<- int
}

func (ts *TestServer) Receive(self PID, inbox <-chan any) error {
	return ts.exec(ts, self, inbox)
}

func testSpawn(t *testing.T, runnable Runnable) PID {
	pid := Spawn(runnable)
	t.Cleanup(func() {
		if IsAlive(pid) {
			Exit(rootPID, pid, exitreason.Kill)
		}
	})
	return pid
}
