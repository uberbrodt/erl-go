/*
Package port provides a [Runnable] used to interact with external system processes (ExtProg).
It uses the [os.Exec.Cmd] subsystem from the stdlib, but wraps it with Processes.
This provides the following guarantees:

 1. A Port must be started by another process. This process will be known as the PortOwner.

 2. When the ExtProg exits, the PortOwner will receive an [erl.ExitMsg] if it is non-zero.
    If the process exits cleanly, it will have [exitreason.Normal] and will be ignored by the PortOwner
    unless it is trapping exits (this is behavior that applies to all processes). However an ExtProg
    returning non-zero exit code will cause the PortOwner
    to crash (if not trapping exits).

 3. If the [ReturnExitStatus] option is set, the PortOwner will receive a [PortExited] msg, regardless
    of the ExtProg exit code.

 4. By default, the stdout of the ExtProg will sent to the PortOwner, line by line, as a [PortMessage].
    Alternative decoding options can be set when opening the port by provding a [bufio.SplitFunc] or
    using one of the existing Decode* Opts in this package. To send this output else where, use
    [SetStdOut] with a [*os.File] or buffer, or [IgnoreStdOut] to ignore output completely.

 5. Data can be sent to the ExtProg via stdin via [PortCommand] messages. It is up to the sender to
    format the messages in a way the ExtProg can parse.

The above features allow for asynchronous management of both long-running and transient ExtProgs, integrated
into a supervision tree.
*/
package port

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"syscall"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
)

type status string

const (
	closing status = "closing"
	closed  status = "closed"
)

type Port struct {
	cmdArg string
	args   []string
	cmd    *exec.Cmd
	opts   Opts
	parent erl.PID
	stdout io.ReadCloser
	stderr io.ReadCloser
	stdin  io.WriteCloser
	closer erl.PID
	status status
}

func (p *Port) kill() {
	// Good little UNIX processes will exit when stdin is closed
	p.stdin.Close()
	// For the bad ones, we drop the hammer
	if p.opts.exitSignal != nil {
		syscall.Kill(-p.cmd.Process.Pid, *p.opts.exitSignal)
	}
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

	if p.opts.stdout != nil {
		p.cmd.Stdout = p.opts.stdout
	} else if p.opts.ignoreStdOut {
		// leaving p.cmd.Stdout equal to nil will send the output to /dev/null
	} else {
		p.stdout, initErr = p.cmd.StdoutPipe()
		if initErr != nil {
			return exitreason.Exception(initErr)
		}
		go func() {
			buf := bufio.NewScanner(p.stdout)
			buf.Split(p.opts.decoder)
			for buf.Scan() {
				erl.Send(p.parent, PortMessage(buf.Bytes()))
			}
		}()
	}

	// by default, stderr is ignored entirely
	if p.opts.stderr != nil {
		p.cmd.Stderr = p.opts.stderr
	} else if p.opts.sendStdErr {
		p.stderr, initErr = p.cmd.StderrPipe()
		if initErr != nil {
			return exitreason.Exception(initErr)
		}
		go func() {
			buf := bufio.NewScanner(p.stderr)
			buf.Split(p.opts.stdErrDecoder)
			for buf.Scan() {
				erl.Send(p.parent, PortErrMessage(buf.Bytes()))
			}
		}()
	}

	err := p.cmd.Start()
	if err != nil {
		return exitreason.Exception(err)
	}

	if p.cmd.Process == nil {
		return exitreason.Exception(fmt.Errorf("%s : %v did not start", p.cmdArg, p.args))
	}

	go func() {
		err := p.cmd.Wait()
		erl.Send(self, PortExited{Err: err, Port: self})
	}()

	for {
		anymsg, ok := <-inbox

		if !ok {
			return nil
		}

		switch msg := anymsg.(type) {
		case erl.ExitMsg:
			if msg.Proc.Equals(p.parent) {
				erl.DebugPrintf("port %v received ExitMsg from parent, killing external process", self)
				p.kill()
				return exitreason.Shutdown("Port exited because the parent process exited")
			}
		case PortCommand:
			erl.Logger.Printf("sending message to port: %s", msg)
			_, err := p.stdin.Write(msg)
			if err != nil {
				p.kill()
				return err
			}
			erl.Logger.Printf("success sending message: %s", msg)
		case closePort:
			p.kill()
			p.status = closing
			p.closer = msg.sender
		case PortExited:
			if p.opts.exitStatus {
				erl.Send(p.parent, msg)
			}

			// if a close was requested, we always return  normal
			if p.status == closing {
				if !p.closer.Equals(p.parent) {
					erl.Send(p.closer, PortClosed{Port: self, Err: msg.Err})
				}
				erl.Send(p.parent, PortClosed{Port: self, Err: msg.Err})
				return exitreason.Normal
			}

			if msg.Err == nil {
				return exitreason.Normal
			} else {
				return exitreason.Exception(msg.Err)
			}
		}

	}
}
