package erl

import (
	"fmt"

	"github.com/uberbrodt/erl-go/erl/exitreason"
)

var rootPID PID

// The idea here is that rootProc will always exist and just log
// messages it gets. Once Supervisor is done, have this restart and always
// keep the rootPID with a valid process.
func init() {
	rootPID = Spawn(&rootProc{})
	ProcessFlag(rootPID, TrapExit, true)
}

type rootProc struct{}

// XXX: What if he dies for some reason? Does he need a supervisor?
func (rp *rootProc) Receive(self PID, inbox <-chan any) error {
	for anymsg := range inbox {
		switch msg := anymsg.(type) {
		case ExitMsg:
			if !msg.Link {
				Logger.Printf("RootPID received an exit signal with reason: %v", msg.Reason)
				return exitreason.Exception(fmt.Errorf("RootPID received an exit signal with reason: %w", msg.Reason))
			}
		default:
			Logger.Printf("rootProc received: %+v", msg)
		}
	}
	return nil
}

func RootPID() PID {
	return rootPID
}
