package genserver_test

import (
	"testing"
	"time"

	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
	"github.com/uberbrodt/erl-go/erl/x/erltest"
	"github.com/uberbrodt/erl-go/erl/x/erltest/testcase"
	"github.com/uberbrodt/erl-go/erl/x/erltest/testserver"
)

// if the process we're starting exits init cleanly, we'll get a nil error
// returned and no exitmsgs to the parent
func TestStartLink_WorkerProcess_Success(t *testing.T) {
	tc := testcase.New(t, erltest.WaitTimeout(3*time.Second))

	var parentPID erl.PID
	var pid erl.PID
	var err error

	tc.Arrange(func(self erl.PID) {
		parentPID = tc.StartServer(func(self erl.PID) (erl.PID, error) {
			return testserver.StartLink(self, testserver.NewConfig().
				AddInfoHandler(erl.DownMsg{}, parentGetsDownMsg(t, self)))
		})

		tc.Receiver().Expect(erl.ExitMsg{}, gomock.Any()).Times(0)
	})

	tc.Act(func() {
		pid, err = testserver.StartLink(parentPID, testserver.NewConfig().SetInit(testserver.InitOK))
	})

	tc.Assert(func() {
		assert.NilError(t, err)
		assert.Assert(t, erl.IsAlive(pid))
		erl.Link(tc.TestPID(), pid)
	})
}

// The parent will get an exit message and Exitsignals will be published to the parent
func TestStartLink_WorkerProcess_Exception(t *testing.T) {
	tc := testcase.New(t, erltest.WaitTimeout(3*time.Second))

	var parentPID erl.PID
	var pid erl.PID
	var err error

	tc.Arrange(func(self erl.PID) {
		parentPID = tc.StartServer(func(self erl.PID) (erl.PID, error) {
			return testserver.StartLink(self, testserver.NewConfig().
				AddInfoHandler(erl.DownMsg{}, parentGetsDownMsg(t, self)))
		})

		tc.Receiver().Expect(erl.ExitMsg{}, gomock.Any()).Times(1)
	})

	tc.Act(func() {
		pid, err = testserver.StartLink(parentPID, testserver.NewConfig().SetInit(testserver.InitError))
	})

	tc.Assert(func() {
		assert.ErrorContains(t, err, "shutdown: exited in init")
		assert.Assert(t, !erl.IsAlive(pid))
	})
}

// the StartLink function will return with exitreason.Ignore. The parent/caller
// should not receive an exit signal because the process should exit with Normal
func TestStartLink_WorkerProcess_Ignore(t *testing.T) {
	tc := testcase.New(t, erltest.WaitTimeout(3*time.Second))

	var parentPID erl.PID
	var pid erl.PID
	var err error

	tc.Arrange(func(self erl.PID) {
		parentPID = tc.StartServer(func(self erl.PID) (erl.PID, error) {
			return testserver.StartLink(self, testserver.NewConfig().
				AddInfoHandler(erl.DownMsg{}, parentGetsDownMsg(t, self)))
		})

		tc.Receiver().Expect(erl.ExitMsg{}, gomock.Any()).Times(0)
	})

	tc.Act(func() {
		pid, err = testserver.StartLink(parentPID, testserver.NewConfig().SetInit(testserver.InitIgnore))
	})

	tc.Assert(func() {
		assert.ErrorIs(t, err, exitreason.Ignore)
		assert.Assert(t, !erl.IsAlive(pid))
	})
}

// if the process we're starting exits init cleanly, we'll get a nil error
// returned and no downmsgs to the parent
func TestStartMonitor_WorkerProcess_Success(t *testing.T) {
	tc := testcase.New(t, erltest.WaitTimeout(3*time.Second))

	var parentPID erl.PID
	var pid erl.PID
	var err error

	tc.Arrange(func(self erl.PID) {
		parentPID = tc.StartServer(func(self erl.PID) (erl.PID, error) {
			return testserver.StartLink(self, testserver.NewConfig().
				AddInfoHandler(erl.DownMsg{}, parentGetsDownMsg(t, self)))
		})

		tc.Receiver().Expect(DownNotification{}, gomock.Any()).Times(0)
	})

	tc.Act(func() {
		pid, _, err = testserver.StartMonitor(parentPID, testserver.NewConfig().SetInit(testserver.InitOK))
	})

	tc.Assert(func() {
		assert.NilError(t, err)
		assert.Assert(t, erl.IsAlive(pid))
		// link this so it's cleaned up when test exits
		erl.Link(tc.TestPID(), pid)
	})
}

// The call to StartMonitor will return the exit reason and the parent proces swill
// receive a downsignal
func TestStartMonitor_WorkerProcess_Exception(t *testing.T) {
	tc := testcase.New(t, erltest.WaitTimeout(3*time.Second))

	var parentPID erl.PID
	var pid erl.PID
	var err error

	tc.Arrange(func(self erl.PID) {
		parentPID = tc.StartServer(func(self erl.PID) (erl.PID, error) {
			return testserver.StartLink(self, testserver.NewConfig().
				AddInfoHandler(erl.DownMsg{}, parentGetsDownMsg(t, self)))
		})

		tc.Receiver().Expect(DownNotification{}, gomock.Any()).Times(1)
	})

	tc.Act(func() {
		pid, _, err = testserver.StartMonitor(parentPID, testserver.NewConfig().SetInit(testserver.InitError))
	})

	tc.Assert(func() {
		assert.Error(t, err, "EXIT{shutdown: exited in init}")
		assert.Assert(t, !erl.IsAlive(pid))
	})
}

// the StartMonitor function will return with exitreason.Ignore. The parent/caller
// will receive a DownMsg.
func TestStartMonitor_WorkerProcess_Ignore(t *testing.T) {
	tc := testcase.New(t, erltest.WaitTimeout(3*time.Second))

	var parentPID erl.PID
	var pid erl.PID
	var err error

	tc.Arrange(func(self erl.PID) {
		parentPID = tc.StartServer(func(self erl.PID) (erl.PID, error) {
			return testserver.StartLink(self, testserver.NewConfig().
				AddInfoHandler(erl.DownMsg{}, parentGetsDownMsg(t, self)))
		})

		tc.Receiver().Expect(DownNotification{}, gomock.Any()).Times(1)
	})

	tc.Act(func() {
		pid, _, err = testserver.StartMonitor(parentPID, testserver.NewConfig().SetInit(testserver.InitIgnore))
	})

	tc.Assert(func() {
		assert.ErrorIs(t, err, exitreason.Ignore)
		assert.Assert(t, !erl.IsAlive(pid))
	})
}

func TestStart_WorkerProcess_Success(t *testing.T) {
	tc := testcase.New(t, erltest.WaitTimeout(3*time.Second))

	var parentPID erl.PID
	var pid erl.PID
	var err error

	tc.Arrange(func(self erl.PID) {
		parentPID = tc.StartServer(func(self erl.PID) (erl.PID, error) {
			return testserver.StartLink(self, testserver.NewConfig().
				AddInfoHandler(erl.DownMsg{}, parentGetsDownMsg(t, self)))
		})

		tc.Receiver().Expect(erl.ExitMsg{}, gomock.Any()).Times(0)
	})

	tc.Act(func() {
		pid, err = testserver.Start(parentPID, testserver.NewConfig().SetInit(testserver.InitOK))
	})

	tc.Assert(func() {
		assert.NilError(t, err)
		assert.Assert(t, erl.IsAlive(pid))
		erl.Link(tc.TestPID(), pid)
	})
}

func TestStart_WorkerProcess_Exception(t *testing.T) {
	tc := testcase.New(t, erltest.WaitTimeout(3*time.Second))

	var parentPID erl.PID
	var pid erl.PID
	var err error

	tc.Arrange(func(self erl.PID) {
		parentPID = tc.StartServer(func(self erl.PID) (erl.PID, error) {
			return testserver.StartLink(self, testserver.NewConfig().
				AddInfoHandler(erl.DownMsg{}, parentGetsDownMsg(t, self)))
		})

		tc.Receiver().Expect(erl.ExitMsg{}, gomock.Any()).Times(0)
		tc.Receiver().Expect(DownNotification{}, gomock.Any()).Times(0)
	})

	tc.Act(func() {
		pid, err = testserver.Start(parentPID, testserver.NewConfig().SetInit(testserver.InitError))
	})

	tc.Assert(func() {
		assert.Error(t, err, "EXIT{shutdown: exited in init}")
		assert.Assert(t, !erl.IsAlive(pid))
	})
}

func TestStart_WorkerProcess_Ignore(t *testing.T) {
	tc := testcase.New(t, erltest.WaitTimeout(3*time.Second))

	var parentPID erl.PID
	var pid erl.PID
	var err error

	tc.Arrange(func(self erl.PID) {
		parentPID = tc.StartServer(func(self erl.PID) (erl.PID, error) {
			return testserver.StartLink(self, testserver.NewConfig().
				AddInfoHandler(erl.DownMsg{}, parentGetsDownMsg(t, self)))
		})

		tc.Receiver().Expect(erl.ExitMsg{}, gomock.Any()).Times(0)
		tc.Receiver().Expect(DownNotification{}, gomock.Any()).Times(0)
	})

	tc.Act(func() {
		pid, err = testserver.Start(parentPID, testserver.NewConfig().SetInit(testserver.InitIgnore))
	})

	tc.Assert(func() {
		assert.ErrorIs(t, err, exitreason.Ignore)
		assert.Assert(t, !erl.IsAlive(pid))
	})
}
