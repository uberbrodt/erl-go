package port

import (
	"bufio"
	"io"
	"syscall"

	"github.com/uberbrodt/erl-go/erl"
)

type ShutdownReason string

const (
	PortExitedReason = "port exited"
	PortClosedReason = "port closed"
)

// returned when the port process returns and [ReturnExitStatus] is true.
// if [err] is not nil, it may be an [*os/exec.ExitError]
type PortExited struct {
	Port erl.PID
	Err  error
}

type closePort struct {
	sender erl.PID
}

type PortClosed struct {
	Port erl.PID
	// the exit error of the ExtProg, if any
	Err error
}

type PortCommand []byte

// decocded data from the ExtProg stdout
type PortMessage []byte

// Received from a port error stream
type PortErrMessage []byte

type PortCmd struct {
	cmd  string
	args []string
}

func NewPortCmd(cmd string, args ...string) PortCmd {
	return PortCmd{cmd, args}
}

// Will read [num] bytes from the external command before returning
// a message to the port owner.
func DecodeNumBytes(num int) Opt {
	return func(opts Opts) Opts {
		opts.decoder = DecodeNumBytesSplitFun(num)
		return opts
	}
}

func CustomDecoder(splitFun bufio.SplitFunc) Opt {
	return func(opts Opts) Opts {
		opts.decoder = splitFun
		return opts
	}
}

// Port messages will be parsed by \n
func DecodeLines() Opt {
	return func(opts Opts) Opts {
		opts.decoder = DecodeLinesSplitFun
		return opts
	}
}

// Port messages will be parsed as NUL delimited strings
func DecodeNULs() Opt {
	return func(opts Opts) Opts {
		opts.decoder = DecodeNULSplitFun
		return opts
	}
}

// This will cause the exit status to be returend to the PortOwner
func ReturnExitStatus() Opt {
	return func(opts Opts) Opts {
		opts.exitStatus = true
		return opts
	}
}

// Redirect stdout to an [*os.File] or buffer. A buffer or file
// should be safe for other processes to read after the [erl.ExitMsg] is
// received for the port
func SetStdOut(stdout io.Writer) Opt {
	return func(opts Opts) Opts {
		opts.stdout = stdout
		return opts
	}
}

// Causing stdout to be redirected to a null device.
// The PortOwner should recieve no [PortMessage]s while this is set.
func IgnoreStdOut() Opt {
	return func(opts Opts) Opts {
		opts.ignoreStdOut = true
		return opts
	}
}

// Redirect ExtProg stderr stream to this item. Works the same way as [SetStdOut]
func SetStdErr(stderr io.Writer) Opt {
	return func(opts Opts) Opts {
		opts.stderr = stderr
		return opts
	}
}

// If set, the PortOwner will receive the data from the ExtProg error stream
// as [PortErrMessage]s. Not all programs will return error messages in the saem
// format as stdout, this option accepts a distinct bufio.Splitfunc to use for stderr.
func ReceiveStdErr(splitFunc bufio.SplitFunc) Opt {
	return func(opts Opts) Opts {
		opts.sendStdErr = true
		opts.stdErrDecoder = splitFunc
		return opts
	}
}

// If set, this signal will be sent to the ExtProg during
// port shutdown in addition to closing stdin
func SetExitSignal(sig syscall.Signal) Opt {
	return func(opts Opts) Opts {
		opts.exitSignal = &sig
		return opts
	}
}

type Opts struct {
	decoder       bufio.SplitFunc
	stdErrDecoder bufio.SplitFunc
	// if true, return the process exit status to the caller
	exitStatus   bool
	stdout       io.Writer
	ignoreStdOut bool
	// if true, redirect the ExtProg error stream to PortOwner
	sendStdErr bool
	stderr     io.Writer
	exitSignal *syscall.Signal
}

type Opt func(opts Opts) Opts

func buildOpts(opts []Opt) Opts {
	defaults := Opts{
		decoder: bufio.ScanLines,
	}

	for _, opt := range opts {
		defaults = opt(defaults)
	}

	return defaults
}

// Opens a port.
//
// TODO: list all options here.
func Open(self erl.PID, cmd PortCmd, opts ...Opt) erl.PID {
	optS := buildOpts(opts)
	p := &Port{
		cmdArg: cmd.cmd,
		args:   cmd.args,
		parent: self,
		opts:   optS,
	}
	return erl.Spawn(p)
}

// Closes a port. A [PortExited] message will be sent to the
// sender and the PortOwner
func Close(self erl.PID, port erl.PID) {
	erl.Send(port, closePort{sender: self})
}

// sends an asynchronous command to the port which will be sent to the
// external procs stdin. It's the callers responsibility to encode [msg]
// to a format the external program understands.
func Cast(port erl.PID, msg []byte) {
	erl.Send(port, PortCommand(msg))
}

// TODO: implement a Connect method to take over a port
