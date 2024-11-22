package inbox_test

import (
	"testing"

	"github.com/uberbrodt/erl-go/erl/internal/inbox"
	"gotest.tools/v3/assert"
)

func TestPop_ReturnsValue(t *testing.T) {
	ibox := inbox.New[int]()

	ibox.Enqueue(12)

	result, ok, _ := ibox.Pop()

	assert.Assert(t, ok)

	assert.Equal(t, result, 12)
}

func TestPop_RemovesItemFromInbox(t *testing.T) {
	ibox := inbox.New[int]()

	ibox.Enqueue(12)
	ibox.Enqueue(37)

	result, ok, _ := ibox.Pop()

	assert.Assert(t, ok)

	assert.Equal(t, result, 12)

	assert.Equal(t, ibox.Size(), 1)
}

func TestPop_ReturnsNothing(t *testing.T) {
	ibox := inbox.New[int]()

	result, ok, _ := ibox.Pop()

	assert.Assert(t, !ok)

	assert.Equal(t, result, 0)
}

func TestSize_ReturnsNumberOfItems(t *testing.T) {
	ibox := inbox.New[int]()

	ibox.Enqueue(12)
	ibox.Enqueue(37)
	ibox.Enqueue(92)

	result := ibox.Size()

	assert.Equal(t, result, 3)
}
