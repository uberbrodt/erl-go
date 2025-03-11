/*
the [testcase] is intended to structure tests using `erltest`.

The [Case] object contains three methods--[Case.Arrange], [Case.Act] and [Case.Assert]--that are intended to be called
in order.

# Example


	tc := testcase.New(t, erltest.WaitTimeout(3*time.Second))

	var parentPID erl.PID
	var pid erl.PID
	var err error
    // put expectations or other setup code here
	tc.Arrange(func(self erl.PID) {
		parentPID = tc.StartServer(func(self erl.PID) (erl.PID, error) {
			return testserver.StartLink(self, testserver.NewConfig().
				AddInfoHandler(erl.DownMsg{}, parentGetsDownMsg(t, self)))
		})

		tc.Receiver().Expect(erl.ExitMsg{}, gomock.Any()).Times(0)
	})

    // execute the system under test
	tc.Act(func() {
		pid, err = testserver.StartLink(parentPID, testserver.NewConfig().SetInit(testserver.InitOK))
	})

    // Any assertions that should be run AFTER the TestReceiver expectations have passed or any other Waitables.
	tc.Assert(func() {
		assert.NilError(t, err)
		assert.Assert(t, erl.IsAlive(pid))
		erl.Link(tc.TestPID(), pid)
	})

*/

package testcase

import (
	"sync"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
	"github.com/uberbrodt/erl-go/erl/x/erltest"
)

type Waitable interface {
	Wait()
}

type Case struct {
	t         *testing.T
	tr        *erltest.TestReceiver
	self      erl.PID
	waitables []Waitable
}

func New(t *testing.T, opts ...erltest.ReceiverOpt) *Case {
	self, tr := erltest.NewReceiver(t, opts...)

	return &Case{
		t:         t,
		tr:        tr,
		self:      self,
		waitables: []Waitable{tr},
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
	var wg sync.WaitGroup
	wg.Add(len(c.waitables))
	for _, waitable := range c.waitables {
		go func() {
			waitable.Wait()
			wg.Done()
		}()
	}
	wg.Wait()
	f()
}

// Add a dependency that must return before the [Assert] is called. Usually this is
// another [TestReceiver]
func (c *Case) WaitOn(w Waitable) {
	c.waitables = append(c.waitables, w)
}

// Starts a server and links it to the testcase test pid. Will fail the test if startLink
// returns an error. Since it is linked to the test process, it should shutdown with it.
func (c *Case) StartServer(startLink func(self erl.PID) (erl.PID, error)) erl.PID {
	c.t.Helper()
	pid, err := startLink(c.TestPID())

	assert.NilError(c.t, err)

	return pid
}

// Spawns a process that will receive an exit msg after the test ends
func (c *Case) Spawn(r erl.Runnable) erl.PID {
	pid := erl.Spawn(r)

	c.t.Cleanup(func() {
		erl.Exit(c.TestPID(), pid, exitreason.TestExit)
	})
	return pid
}
