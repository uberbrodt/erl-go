package supervisor

import (
	"testing"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
	"github.com/uberbrodt/erl-go/erl/genserver"
)

type TestSup struct {
	childSpecs []ChildSpec
	supFlags   SupFlagsS
	ignore     bool
}

func (s TestSup) Init(self erl.PID, args any) InitResult {
	return InitResult{s.supFlags, s.childSpecs, s.ignore}
}

type TestSrvState struct {
	id string
}

func testStartSupervisor(t *testing.T, sup Supervisor, args any) (erl.PID, error) {
	pid, err := StartLink(erl.RootPID(), sup, args, SetName(supName))

	t.Cleanup(func() {
		t.Logf("Cleanup func stopping test supervisor: %v", pid)
		genserver.Stop(erl.RootPID(), pid, genserver.StopReason(exitreason.SupervisorShutdown))
	})
	return pid, err
}
