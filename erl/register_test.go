package erl

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestRegister_ReturnsOK(t *testing.T) {
	pid := testSpawn(t, &TestRunnable{t: t, expected: "foo"})

	result := Register(Name("my_pid"), pid)

	assert.Assert(t, result == nil)
}

func TestRegister_NameInUse(t *testing.T) {
	t.Skip("moved to TestRegistration_NameInUse")
	// NOTE: moved to register_ext_test.go as TestRegistration_ReRegisterNameAfterDownMsg()
	pid1 := testSpawn(t, &TestRunnable{t: t, expected: "foo"})
	pid2 := testSpawn(t, &TestRunnable{t: t, expected: "foo"})

	result := Register(Name("my_pid"), pid1)

	t.Logf("registration result: %+v", result)
	assert.Assert(t, result == nil)

	result = Register(Name("my_pid"), pid2)

	assert.Equal(t, result.Kind, NameInUse)
}

func TestRegister_InvalidNames(t *testing.T) {
	// NOTE: moved to register_ext_test.go as TestRegister_InvalidNames()
	name1 := Name("")
	name2 := Name("undefined")
	name3 := Name("nil")
	pid1 := testSpawn(t, &TestRunnable{t: t, expected: "foo"})

	result := Register(name1, pid1)
	assert.Assert(t, result.Kind == BadName)
	result = Register(name2, pid1)
	assert.Assert(t, result.Kind == BadName)
	result = Register(name3, pid1)
	assert.Assert(t, result.Kind == BadName)
}

func TestRegister_AlreadyRegistered(t *testing.T) {
	// NOTE: moved to register_ext_test.go as TestRegistration_AlreadyRegistered()
	pid := testSpawn(t, &TestRunnable{t: t, expected: "foo"})

	result := Register(Name("my_pid"), pid)

	assert.Assert(t, result == nil)

	result = Register(Name("my_pid"), pid)

	assert.Equal(t, result.Kind, AlreadyRegistered)
}

func TestRegister_BadPidReturnsNoProc(t *testing.T) {
	// NOTE: moved to register_ext_test.go as TestRegistration_BadPidReturnsNoProc()
	result := Register(Name("my_pid"), PID{})

	assert.Equal(t, result.Kind, NoProc)
}

func TestWhereIs_NameFound(t *testing.T) {
	// NOTE: moved to register_ext_test.go as TestRegistration_BadPidReturnsNoProc()
	name := Name("my_pid")
	pid := testSpawn(t, &TestRunnable{t: t, expected: "foo"})
	Register(name, pid)

	foundpid, exists := WhereIs(name)

	assert.Equal(t, pid, foundpid)
	assert.Equal(t, exists, true)
}

func TestWhereIs_NameNotFound(t *testing.T) {
	name := Name("my_pid")
	pid := testSpawn(t, &TestRunnable{t: t, expected: "foo"})
	Register(Name("foo"), pid)

	_, exists := WhereIs(name)

	assert.Assert(t, !exists)
}

func TestUnregister_NameNotFound(t *testing.T) {
	name := Name("my_pid")
	pid := testSpawn(t, &TestRunnable{t: t, expected: "foo"})
	Register(Name("foo"), pid)

	result := Unregister(name)

	assert.Assert(t, !result)
}

// func TestRegistration_NameIsUnregisteredBeforeExitMsgSent(t *testing.T) {
// 	// NOTE: moved to register_ext_test.go as TestRegistration_ReRegisterNameAfterExitMsg()
// 	name := Name("my_pid")
// 	trPID, receiver := NewTestReceiver(t)
// 	pid := testSpawn(t, &TestRunnable{t: t, expected: "foo"})
// 	Link(trPID, pid)
// 	Register(name, pid)
//
// 	registeredPID, ok := WhereIs(name)
// 	assert.Assert(t, ok)
// 	assert.Assert(t, pid.Equals(registeredPID))
//
// 	t.Logf("telling process to exit")
// 	Exit(RootPID(), pid, exitreason.To(exitreason.Shutdown("stop")))
// 	success := receiver.Loop(func(msg any) bool {
// 		xit, ok := msg.(ExitMsg)
// 		if !ok {
// 			// keep looking
// 			return false
// 		}
//
// 		if xit.Proc.Equals(pid) {
// 			// found exit msg for the process we killed, return success
// 			return true
// 		} else {
// 			t.Logf("got an exit message but %v does not match %v", xit.Proc, pid)
// 		}
// 		return false
// 	})
//
// 	assert.Assert(t, success)
// 	t.Logf("check if still alive")
// 	assert.Assert(t, !IsAlive(pid))
//
// 	t.Logf("test done")
// 	// result := Unregister(name)
//
// 	// assert.Assert(t, !result)
// }

// func TestRegistration_NameIsUnregisteredBeforeDownSignalSent(t *testing.T) {
// 	// duplicate in spirit of
// 	// NOTE: moved to register_ext_test.go as TestRegistration_ReRegisterNameAfterDownMsg()
// 	name := Name("my_pid")
// 	trPID, receiver := NewTestReceiver(t)
// 	pid := testSpawn(t, &TestRunnable{t: t, expected: "foo"})
// 	ref := Monitor(trPID, pid)
// 	Register(name, pid)
//
// 	registeredPID, ok := WhereIs(name)
// 	assert.Assert(t, ok)
// 	assert.Assert(t, pid.Equals(registeredPID))
//
// 	Exit(RootPID(), pid, exitreason.To(exitreason.Shutdown("stop")))
// 	success := receiver.Loop(func(msg any) bool {
// 		xit, ok := msg.(DownMsg)
// 		if !ok {
// 			// keep looking
// 			return false
// 		}
//
// 		if xit.Proc.Equals(pid) && xit.Ref == ref {
// 			// found exit msg for the process we killed, return success
// 			return true
// 		} else {
// 			t.Logf("got a DownMsg but %v does not match %v", xit.Proc, pid)
// 		}
// 		return false
// 	})
//
// 	assert.Assert(t, success)
// 	t.Logf("check if still alive")
// 	assert.Assert(t, !IsAlive(pid))
//
// 	t.Logf("test done")
// }
