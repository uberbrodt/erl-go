package erl

import (
	"testing"

	"gotest.tools/v3/assert"

	"github.com/uberbrodt/erl-go/chronos"
	"github.com/uberbrodt/erl-go/erl/exitreason"
)

func TestTimer_Cancel(t *testing.T) {
	self, tr := NewTestReceiver(t)

	tref := SendAfter(self, "timer_ran", chronos.Dur("500ms"))
	cancelErr := CancelTimer(tref)

	var msg any
	result := tr.LoopFor(chronos.Dur("500ms"), func(anymsg any) bool {
		msg = anymsg

		return true
	})

	assert.Assert(t, msg == nil)
	assert.Assert(t, cancelErr == nil)
	assert.ErrorIs(t, result, exitreason.Timeout)
}
