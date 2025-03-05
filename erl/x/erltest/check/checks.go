/*
* This package provides "assertions" for use in [erltest.TestExpectation]s.
*
* # Why is this needed?
*
* [testing.T.FailNow] requires that it be called from the goroutine running the test. Since
* an erl process is by definition not going to execute in the test goroutine, we need to provide
* alternative functions that can be used. This is basically an inversion of [gotest.tools/v3/assert],
* but replacing everything with Checks/returning a bool.
 */
package check

import (
	"testing"

	gocmp "github.com/google/go-cmp/cmp"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
)

// returns false if any item in [checks] fails.
func Chain(t *testing.T, checks ...bool) bool {
	t.Helper()
	for idx, check := range checks {
		if !check {
			t.Logf("[check.Chain] check #%d failed\n", idx)
			return check

		}
	}
	return true
}

// accepts binary comparisons or booleans and returns the result
func Assert(t *testing.T, comparison assert.BoolOrComparison, msgAndArgs ...any) bool {
	t.Helper()
	return assert.Check(t, comparison, msgAndArgs...)
}

// Compares two values using [go-cmp/cmp]
func DeepEqual(t *testing.T, actual, expected any, opts ...gocmp.Option) bool {
	t.Helper()
	return assert.Check(t, cmp.DeepEqual(actual, expected, opts...))
}

func Equal(t *testing.T, actual, expected any, msgAndArgs ...any) bool {
	t.Helper()
	return assert.Check(t, cmp.Equal(actual, expected), msgAndArgs...)
}

// Works with finding an item in a collection OR a substring (uses [strings.Contains] under the hood)
func Contains(t *testing.T, collection any, item any, msgAndArgs ...any) bool {
	t.Helper()
	return assert.Check(t, cmp.Contains(collection, item), msgAndArgs...)
}

// returns true if [ptr] is nil
func Nil(t *testing.T, ptr any, msgAndArgs ...any) bool {
	t.Helper()
	return assert.Check(t, cmp.Nil(ptr), msgAndArgs...)
}

// returns true if error [e] is nil
func NilError(t *testing.T, e error, msgAndArgs ...any) bool {
	t.Helper()
	return assert.Check(t, e == nil, msgAndArgs...)
}

// returns true if [e] is error and it's Error method matches
// [expected]
func Error(t *testing.T, e error, expected string, msgAndArgs ...any) bool {
	t.Helper()
	return assert.Check(t, cmp.Error(e, expected), msgAndArgs...)
}

// returns true if [e] is error and it matches the [expected] error
func ErrorIs(t *testing.T, actual error, expected error) bool {
	t.Helper()
	return assert.Check(t, cmp.ErrorIs(actual, expected))
}

// returns true if [e] is error and [expected] is a substring of [e.Error]
func ErrorContains(t *testing.T, e error, expected string, msgAndArgs ...any) bool {
	t.Helper()
	return assert.Check(t, cmp.ErrorContains(e, expected), msgAndArgs...)
}
