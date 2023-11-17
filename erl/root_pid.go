package erl

var rootPID PID

// The idea here is that rootProc will always exist and just log
// messages it gets. Once Supervisor is done, have this restart and always
// keep the rootPID with a valid process.
func init() {
	rootPID = Spawn(&rootProc{})
	ProcessFlag(rootPID, TrapExit, true)
}

type rootProc struct{}

// TODO: What if he dies for some reason? Does he need a supervisor?
func (rp *rootProc) Receive(self PID, inbox <-chan any) error {
	for {
		msg := <-inbox

		Logger.Printf("rootProc received: %+v", msg)

	}
}

func RootPID() PID {
	return rootPID
}
