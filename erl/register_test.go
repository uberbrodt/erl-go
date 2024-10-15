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

func TestWhereIs_NameNotFound(t *testing.T) {
	name := Name("my_pid")
	pid := testSpawn(t, &TestRunnable{t: t, expected: "foo"})
	Register(Name("foo"), pid)

	_, exists := WhereIs(name)

	assert.Assert(t, !exists)
}
