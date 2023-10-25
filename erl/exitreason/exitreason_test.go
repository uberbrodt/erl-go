package exitreason

import (
	"errors"
	"fmt"
	"testing"

	"gotest.tools/v3/assert"
)

func TestIsNormal_WrappedNormal(t *testing.T) {
	e := fmt.Errorf("My Error: %w", Normal)

	assert.Assert(t, IsNormal(e))
}

func TestIsShutdown_Wrapped(t *testing.T) {
	e := Shutdown("I'm tired")

	assert.Assert(t, IsShutdown(e))

	exre := errors.Unwrap(e)

	exr, ok := exre.(*S)

	var myErr *S

	assert.Assert(t, errors.As(e, &myErr))
	assert.Assert(t, myErr.ShutdownReason() != nil)
	assert.Assert(t, myErr.ShutdownReason().(string) == "I'm tired")

	assert.Assert(t, ok)
	assert.Assert(t, exr.short == "shutdown")
}

func TestIfErrorIsAnyS(t *testing.T) {
	e := Shutdown("whatever")

	rawE := errors.New("whatever")

	var myErr *S

	assert.Assert(t, !errors.Is(e, &S{}))
	assert.Assert(t, errors.As(e, &myErr))

	assert.Assert(t, !errors.As(rawE, &myErr))
}

func TestIsExitReason(t *testing.T) {
	e := Shutdown("whatever")

	err := IsExitReason(e)

	assert.Assert(t, err != nil)
	assert.Assert(t, errors.Is(e, shutdownErr))

	assert.Assert(t, IsExitReason(errors.New("boogada")) == nil)
}

func TestIsException_Wrapped(t *testing.T) {
	err := errors.New("I'm tired")
	e := Exception(err)

	assert.Assert(t, IsException(e))

	exre := errors.Unwrap(e)

	exr, ok := exre.(*S)

	var myErr *S

	assert.Assert(t, errors.As(e, &myErr))
	assert.Assert(t, myErr.ExceptionDetail() != nil)
	assert.Assert(t, myErr.ExceptionDetail() == err)

	assert.Assert(t, ok)
	assert.Assert(t, exr.short == exception)
}
