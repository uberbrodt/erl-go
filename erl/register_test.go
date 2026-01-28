package erl

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestRegister_ReturnsOK(t *testing.T) {
	name := Name("47785447-0764-40b8-b711-b7672eb0834e")
	pid := testSpawn(t, &TestRunnable{t: t, expected: "foo"})

	result := Register(name, pid)

	assert.Assert(t, result == nil)
}

func TestWhereIs_NameNotFound(t *testing.T) {
	nameNotUsed := Name("07671fe7-dc99-4f1b-a42d-43c462a14739")
	nameUsed := Name("e74c0f48-883b-4309-a357-b4d48e9f40bb")

	pid := testSpawn(t, &TestRunnable{t: t, expected: "foo"})
	Register(nameUsed, pid)

	_, exists := WhereIs(nameNotUsed)

	assert.Assert(t, !exists)
}

func TestRegistered_ReturnsRegisteredProcesses(t *testing.T) {
	name1 := Name("reg-test-1-a3b2c1d0-1234-5678-9abc-def012345678")
	name2 := Name("reg-test-2-a3b2c1d0-1234-5678-9abc-def012345679")

	pid1 := testSpawn(t, &TestRunnable{t: t, expected: "foo"})
	pid2 := testSpawn(t, &TestRunnable{t: t, expected: "bar"})

	Register(name1, pid1)
	Register(name2, pid2)

	registrations := Registered()

	// Find our registrations in the list (other tests may have registered names too)
	var found1, found2 bool
	for _, reg := range registrations {
		if reg.Name == name1 && reg.PID.Equals(pid1) {
			found1 = true
		}
		if reg.Name == name2 && reg.PID.Equals(pid2) {
			found2 = true
		}
	}

	assert.Assert(t, found1, "name1 not found in Registered()")
	assert.Assert(t, found2, "name2 not found in Registered()")
}

func TestRegisteredCount_ReturnsCorrectCount(t *testing.T) {
	name1 := Name("count-test-1-b4c3d2e1-2345-6789-abcd-ef0123456789")
	name2 := Name("count-test-2-b4c3d2e1-2345-6789-abcd-ef0123456790")

	pid1 := testSpawn(t, &TestRunnable{t: t, expected: "foo"})
	pid2 := testSpawn(t, &TestRunnable{t: t, expected: "bar"})

	// Clean up on test exit
	t.Cleanup(func() {
		Unregister(name1)
		Unregister(name2)
	})

	// Get count before our registrations
	countBefore := RegisteredCount()

	Register(name1, pid1)
	countAfter1 := RegisteredCount()
	assert.Equal(t, countAfter1, countBefore+1, "count should increase by 1 after first register")

	Register(name2, pid2)
	countAfter2 := RegisteredCount()
	assert.Equal(t, countAfter2, countAfter1+1, "count should increase by 1 after second register")

	Unregister(name1)
	countAfter3 := RegisteredCount()
	assert.Equal(t, countAfter3, countAfter2-1, "count should decrease by 1 after first unregister")

	Unregister(name2)
	countAfter4 := RegisteredCount()
	assert.Equal(t, countAfter4, countAfter3-1, "count should decrease by 1 after second unregister")
}

func TestRegistered_ReturnsSnapshotSafeToIterate(t *testing.T) {
	name := Name("snapshot-test-c5d4e3f2-3456-789a-bcde-f01234567890")
	pid := testSpawn(t, &TestRunnable{t: t, expected: "foo"})

	Register(name, pid)

	// Get snapshot
	registrations := Registered()

	// Unregister while we have the snapshot
	Unregister(name)

	// Snapshot should still contain the registration (it's a copy)
	var found bool
	for _, reg := range registrations {
		if reg.Name == name {
			found = true
			break
		}
	}
	assert.Assert(t, found, "snapshot should contain registration even after unregister")

	// But a new call should not contain it
	newRegistrations := Registered()
	var foundInNew bool
	for _, reg := range newRegistrations {
		if reg.Name == name {
			foundInNew = true
			break
		}
	}
	assert.Assert(t, !foundInNew, "new Registered() call should not contain unregistered name")
}
