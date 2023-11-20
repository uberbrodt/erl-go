// WARNING: the port package is experimental and the interfaces may change

// ports are used to interact with external system processes. Basically wraps [os.Exec.Cmd] instance,
// but you get exit signals if the process exits. If the external command closes without error,
// the port process will exit with [exitreason.Normal]
package port

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os/exec"
	"syscall"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
)

type ShutdownReason string

const (
	PortExitedReason = "port exited"
	PortClosedReason = "port closed"
)

func Open(self erl.PID, cmd string, args ...string) erl.PID {
	p := &Port{cmdArg: cmd, args: args, parent: self}
	return erl.Spawn(p)
}

func Close(self erl.PID, port erl.PID) {
	erl.Send(port, closePort{sender: self})
}

type Port struct {
	cmdArg string
	args   []string
	cmd    *exec.Cmd
	parent erl.PID
	stdout io.ReadCloser
	stderr io.ReadCloser
	stdin  io.WriteCloser
}

type portExited struct {
	err error
}

type closePort struct {
	sender erl.PID
}

type PortClosed struct {
	Port erl.PID
}

func (p *Port) Receive(self erl.PID, inbox <-chan any) error {
	erl.ProcessFlag(self, erl.TrapExit, true)
	erl.Link(p.parent, self)
	p.cmd = exec.Command(p.cmdArg, p.args...)
	// Doing this will set the Process Group ID (PGID) of the process
	// to the same as the process. We can then use "-PID" in syscall
	// to kill the entire process group. This prevents zombie processes
	// in the event that our cmd process has children.
	p.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var initErr error

	p.stdin, initErr = p.cmd.StdinPipe()
	if initErr != nil {
		return exitreason.Exception(initErr)
	}
	p.stdout, initErr = p.cmd.StdoutPipe()
	if initErr != nil {
		return exitreason.Exception(initErr)
	}
	p.stderr, initErr = p.cmd.StderrPipe()
	if initErr != nil {
		return exitreason.Exception(initErr)
	}

	err := p.cmd.Start()
	if err != nil {
		return exitreason.Exception(err)
	}

	if p.cmd.Process == nil {
		return exitreason.Exception(fmt.Errorf("%s : %v did not start", p.cmdArg, p.args))
	}

	go func() {
		buf := bufio.NewScanner(p.stdout)
		for buf.Scan() {
			log.Printf("STDOUT: %s", buf.Text())
		}
	}()
	go func() {
		buf := bufio.NewScanner(p.stderr)
		for buf.Scan() {
			log.Printf("STDERR: %s", buf.Text())
		}
	}()

	go func() {
		err := p.cmd.Wait()
		erl.Send(self, portExited{err: err})
	}()

	for {
		anymsg, ok := <-inbox

		if !ok {
			return nil
		}

		switch msg := anymsg.(type) {
		case erl.ExitMsg:
			if msg.Proc.Equals(p.parent) {
				// Good little UNIX processes will exit when stdin is closed
				erl.DebugPrintf("port %v received ExitMsg from parent, killing external process", self)
				p.stdin.Close()
				// For the bad ones, we drop the hammer
				syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
				return exitreason.Shutdown("Port exited because the parent process exited")
			}
		case closePort:
			p.stdin.Close()
			p.cmd.Process.Kill()
			erl.Send(msg.sender, PortClosed{Port: self})
			return exitreason.Shutdown(PortClosedReason)
		case portExited:
			if msg.err == nil {
				return exitreason.Shutdown(PortExitedReason)
			} else {
				return exitreason.Exception(msg.err)
			}
		}

	}
}