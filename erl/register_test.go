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
