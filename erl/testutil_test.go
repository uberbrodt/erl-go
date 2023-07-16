package erl

import (
	"sync"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/uberbrodt/erl-go/erl/tuple"
)

type TestRunnable struct {
	t        *testing.T
	expected string
}

func (tr *TestRunnable) Receive(self PID, incoming <-chan any) error {
	tr.t.Logf("TestRunnable waiting for incoming message")
	m := <-incoming
	tr.t.Logf("TestRunnable received message %+v", m)

	tup, _ := m.(tuple.Tuple)
	wg, actual := tuple.Two[*sync.WaitGroup, string](tup)

	assert.Equal(tr.t, actual, tr.expected)
	wg.Done()
	return nil
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
