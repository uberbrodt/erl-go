package port

import (
	"bytes"
	"errors"
	"io"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/uberbrodt/erl-go/chronos"
	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
	"github.com/uberbrodt/erl-go/erl/projectpath"
)

func TestOpen_WritesToStdOutBuffer(t *testing.T) {
	self, tr := erl.NewTestReceiver(t)
	// f, err := os.OpenFile("testdata/stdout", os.O_RDWR|os.O_CREATE, 0644)

	buf := bytes.NewBuffer([]byte{})
	// assert.NilError(t, err)

	exe := projectpath.Join("erl/port/testdata/ext_line_cmd.sh")
	port := Open(self, NewPortCmd(exe), SetStdOut(buf))

	Cast(port, []byte("foobar\n"))
	<-time.After(chronos.Dur("50ms"))
	Close(self, port)

	gotClose := tr.Loop(func(anymsg any) bool {
		switch anymsg.(type) {
		case PortClosed:
			return true
		default:
			return false
			// do nothing
		}
	})

	assert.Assert(t, gotClose)
	<-time.After(chronos.Dur("1s"))
	dat, err := io.ReadAll(buf)
	assert.NilError(t, err)
	assert.Assert(t, strings.Contains(string(dat), "foobar"))
}

func TestSendsMessageToPortOwner(t *testing.T) {
	self, tr := erl.NewTestReceiver(t)

	exe := projectpath.Join("erl/port/testdata/ext_line_cmd.sh")
	port := Open(self, NewPortCmd(exe))

	Cast(port, []byte("foobar\n"))
	var gotIntroMsg bool
	var receivedMessage PortMessage

	gotClose := tr.Loop(func(anymsg any) bool {
		switch msg := anymsg.(type) {
		case PortMessage:
			if strings.Contains(string(msg), "started ext program") {
				gotIntroMsg = true
				return false
			}
			receivedMessage = msg
			return gotIntroMsg

		default:
			return false
			// do nothing
		}
	})

	assert.Assert(t, gotClose)
	assert.DeepEqual(t, string(receivedMessage), "foobar")
}

func TestSendsMessageToPortOwner_NULDecoder(t *testing.T) {
	self, tr := erl.NewTestReceiver(t)

	exe := projectpath.Join("erl/port/testdata/ext_stream_cmd.sh")
	port := Open(self, NewPortCmd(exe), DecodeNULs())

	Cast(port, []byte("foobar\000"))
	Cast(port, []byte("blah blah blah\000"))
	var gotIntroMsg bool
	var rMsg1 PortMessage
	var rMsg2 PortMessage

	gotClose := tr.Loop(func(anymsg any) bool {
		switch msg := anymsg.(type) {
		case PortMessage:
			if strings.Contains(string(msg), "started ext program") {
				gotIntroMsg = true
				return false
			}
			if len(rMsg1) == 0 {
				rMsg1 = msg
				return false
			}
			if len(rMsg2) == 0 {
				rMsg2 = msg
				return true
			}
			return false

		default:
			return false
			// do nothing
		}
	})

	assert.Assert(t, gotClose)
	assert.Assert(t, gotIntroMsg)
	assert.DeepEqual(t, string(rMsg1), "foobar")
	assert.DeepEqual(t, string(rMsg2), "blah blah blah")
}

func TestSendsMessageToPortOwner_ByteLenDecoder(t *testing.T) {
	self, tr := erl.NewTestReceiver(t)

	exe := projectpath.Join("erl/port/testdata/bo_cmd.sh")
	port := Open(self, NewPortCmd(exe, "8"), DecodeNumBytes(8))

	Cast(port, []byte("foobarak"))
	Cast(port, []byte("blahblahblahblah part"))
	var rMsg1 PortMessage
	var rMsg2 PortMessage
	var rMsg3 PortMessage

	tr.Loop(func(anymsg any) bool {
		switch msg := anymsg.(type) {
		case PortMessage:
			if len(rMsg1) == 0 {
				rMsg1 = msg
				return false
			}
			if len(rMsg2) == 0 {
				rMsg2 = msg
				return false
			}
			if len(rMsg3) == 0 {
				rMsg3 = msg
				return true
			}
			return false

		default:
			return false
		}
	})
	assert.DeepEqual(t, string(rMsg1), "foobarak")
	assert.DeepEqual(t, string(rMsg2), "blahblah")
	assert.DeepEqual(t, string(rMsg3), "blahblah")

	// TODO: test that we get partial messages when external program is closing
	// Close(self, port)
	// var rMsg4 PortMessage
	// tr.Loop(func(anymsg any) bool {
	// 	switch msg := anymsg.(type) {
	// 	case PortMessage:
	// 		rMsg4 = msg
	// 		return true
	// 	default:
	// 		return false
	// 	}
	// })
	//
	// assert.DeepEqual(t, string(rMsg4), " part")
}

func TestOpts_ReturnExitStatus_WhenZero(t *testing.T) {
	self, tr := erl.NewTestReceiver(t)

	exe := projectpath.Join("erl/port/testdata/fail_after_time.sh")
	port := Open(self, NewPortCmd(exe, "3", "0"), ReturnExitStatus())

	var gotIntroMsg bool
	var msgCnt int
	var portExit PortExited
	tr.Loop(func(anymsg any) bool {
		switch msg := anymsg.(type) {
		case PortMessage:
			if strings.Contains(string(msg), "started") {
				gotIntroMsg = true
				return false
			}
			if strings.Contains(string(msg), "loop") {
				msgCnt++
			}
			return false
		case PortExited:
			portExit = msg
			return true

		default:
			return false
			// do nothing
		}
	})

	assert.Assert(t, gotIntroMsg)
	assert.Assert(t, portExit.Err == nil)
	assert.Assert(t, portExit.Port.Equals(port))
}

func TestOpts_ReturnExitStatus_WhenNonZero(t *testing.T) {
	self, tr := erl.NewTestReceiver(t)

	exe := projectpath.Join("erl/port/testdata/fail_after_time.sh")
	port := Open(self, NewPortCmd(exe, "3", "1"), ReturnExitStatus())

	var gotIntroMsg bool
	var msgCnt int
	var portExit PortExited
	var exitMsg erl.ExitMsg
	tr.Loop(func(anymsg any) bool {
		switch msg := anymsg.(type) {
		case PortMessage:
			if strings.Contains(string(msg), "started") {
				gotIntroMsg = true
				return false
			}
			if strings.Contains(string(msg), "loop") {
				msgCnt++
			}
			return false
		case PortExited:
			portExit = msg
			return false
		case erl.ExitMsg:
			exitMsg = msg
			return true

		default:
			return false
			// do nothing
		}
	})

	var execErr *exec.ExitError
	assert.Assert(t, gotIntroMsg)
	assert.Assert(t, portExit.Err != nil)
	assert.Assert(t, exitreason.IsException(exitMsg.Reason))
	assert.Assert(t, errors.As(portExit.Err, &execErr))
	assert.Equal(t, execErr.ExitCode(), 1)
	assert.Assert(t, portExit.Port.Equals(port))
}

func TestOpts_IgnoreStdOut(t *testing.T) {
	self, tr := erl.NewTestReceiver(t)

	exe := projectpath.Join("erl/port/testdata/fail_after_time.sh")
	port := Open(self, NewPortCmd(exe, "3", "0"), ReturnExitStatus(), IgnoreStdOut())

	var gotIntroMsg bool
	var msgCnt int
	var portExit PortExited
	tr.Loop(func(anymsg any) bool {
		switch msg := anymsg.(type) {
		case PortMessage:
			if strings.Contains(string(msg), "started") {
				gotIntroMsg = true
				return false
			}
			if strings.Contains(string(msg), "loop") {
				msgCnt++
			}
			return false
		case PortExited:
			portExit = msg
			return true

		default:
			return false
			// do nothing
		}
	})

	assert.Assert(t, !gotIntroMsg)
	assert.Assert(t, msgCnt == 0)
	assert.Assert(t, portExit.Port.Equals(port))
}

func TestOpts_SetExitSignal(t *testing.T) {
	self, tr := erl.NewTestReceiver(t)

	exe := projectpath.Join("erl/port/testdata/fail_after_time.sh")
	port := Open(self, NewPortCmd(exe, "3", "2"), ReturnExitStatus(), SetExitSignal(syscall.SIGINT))

	var gotIntroMsg bool
	var gotSigMsg bool
	var portExit PortExited
	tr.Loop(func(anymsg any) bool {
		switch msg := anymsg.(type) {
		case PortMessage:
			if strings.Contains(string(msg), "started") {
				gotIntroMsg = true
				Close(self, port)
				return false
			}
			if strings.Contains(string(msg), "SIGINT") {
				gotSigMsg = true
				return true
			}
			return false
		case PortExited:
			portExit = msg
			return true

		default:
			return false
			// do nothing
		}
	})

	assert.Assert(t, gotIntroMsg)
	assert.Assert(t, portExit.Err == nil)
	assert.Assert(t, gotSigMsg)
}

func TestOpts_ReceiveStderr_GetPortErrMessages(t *testing.T) {
	self, tr := erl.NewTestReceiver(t)

	exe := projectpath.Join("erl/port/testdata/echo_stderr.sh")
	Open(self, NewPortCmd(exe, "3", "0"), ReturnExitStatus(), ReceiveStdErr(DecodeLinesSplitFun))

	var errMsgCnt int
	tr.Loop(func(anymsg any) bool {
		switch msg := anymsg.(type) {
		case PortErrMessage:
			if strings.Contains(string(msg), "loop") {
				errMsgCnt++
			}
			return false
		case PortExited:
			return true

		default:
			return false
			// do nothing
		}
	})

	assert.Equal(t, errMsgCnt, 4)
}
