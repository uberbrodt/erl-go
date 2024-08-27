package testcase

import (
	"testing"

	"gotest.tools/v3/assert"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/erltest"
	"github.com/uberbrodt/erl-go/erl/exitreason"
)

type Case struct {
	t    *testing.T
	tr   *erltest.TestReceiver
	self erl.PID
}

func New(t *testing.T, opts ...erltest.ReceiverOpt) *Case {
	self, tr := erltest.NewReceiver(t, opts...)

	return &Case{
		t:    t,
		tr:   tr,
		self: self,
	}
}

func (c *Case) TestPID() erl.PID {
	return c.self
}

func (c *Case) Receiver() *erltest.TestReceiver {
	return c.tr
}

func (c *Case) Arrange(f func(self erl.PID)) {
	f(c.self)
}

func (c *Case) Act(f func()) {
	f()
}

func (c *Case) Assert(f func()) {
	c.tr.Wait()
	f()
}

// Starts a server and links it to the testcase test pid. Will fail the test if startLink
// returns an error. Since it is linked to the test process, it should shutdown with it.
func (c *Case) StartServer(startLink func(self erl.PID) (erl.PID, error)) erl.PID {
	c.t.Helper()
	pid, err := startLink(c.TestPID())

	assert.NilError(c.t, err)

	return pid
}

func (c *Case) Spawn(r erl.Runnable) erl.PID {
	pid := erl.Spawn(r)

	c.t.Cleanup(func() {
		erl.Exit(c.TestPID(), pid, exitreason.TestExit)
	})
	return pid
}
