package inbox_test

import (
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/uberbrodt/erl-go/erl/internal/inbox"
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

func TestBlockingPop(t *testing.T) {
	sig := make(chan int)
	ended := make(chan struct{})

	ibox := inbox.New[int]()

	go func() {
		t.Logf("%s - consumer goroutine started", nanoNow())
		for {
			item, ok, closed := ibox.BlockingPop()
			if closed != nil {
				t.Logf("%s - inbox closed", nanoNow())
				close(ended)
				return

			}
			if !ok {
				t.Logf("someone beat us to it, try again")
				continue
			}
			t.Logf("%s - got value: %d", nanoNow(), item)
			sig <- item
		}
	}()

	t.Logf("%s - enqueue item 1", nanoNow())
	ibox.Enqueue(1)
	t.Logf("%s - enqueue item 2", nanoNow())
	ibox.Enqueue(2)
	t.Logf("%s - close inbox", nanoNow())

	t.Logf("%s - waiting for item 1", nanoNow())
	item1 := <-sig
	assert.Equal(t, item1, 1)

	t.Logf("%s - waiting for item 2", nanoNow())
	item2 := <-sig
	assert.Equal(t, item2, 2)
	ibox.Close()
	<-ended
}

func TestIter(t *testing.T) {
	sig := make(chan int)
	ended := make(chan struct{})

	ibox := inbox.New[int]()

	go func() {
		t.Logf("%s - iteration goroutine started\n", nanoNow())
		for item, ok := range ibox.Iter() {
			t.Logf("%s - loop run ibox.Iter()\n", nanoNow())
			if !ok {
				t.Logf("%s - someone beat us to it, try again\n", nanoNow())
				continue
			}
			t.Logf("%s - got value: %d\n", nanoNow(), item)
			sig <- item
		}
		close(ended)
	}()

	t.Logf("%s - enqueue item 1\n", nanoNow())
	ibox.Enqueue(1)
	t.Logf("%s - enqueue item 2\n", nanoNow())
	ibox.Enqueue(2)

	t.Logf("%s - waiting for item 1\n", nanoNow())
	item1 := <-sig
	assert.Equal(t, item1, 1)

	t.Logf("%s -waiting for item 2\n", nanoNow())
	item2 := <-sig
	assert.Equal(t, item2, 2)
	ibox.Close()
	<-ended
}

func TestChannel(t *testing.T) {
	sig := make(chan int)
	ended := make(chan struct{})

	ibox := inbox.New[int]()

	go func() {
		c := ibox.Channel()
		for item := range c {
			t.Logf("%s - got value: %d", nanoNow(), item)
			sig <- item
		}
		close(ended)
	}()

	t.Logf("%s - enqueue item 1\n", nanoNow())
	ibox.Enqueue(1)
	t.Logf("%s - enqueue item 2\n", nanoNow())
	ibox.Enqueue(2)

	t.Logf("%s - waiting for item 1", nanoNow())
	item1 := <-sig
	assert.Equal(t, item1, 1)

	t.Logf("%s - waiting for item 2", nanoNow())
	item2 := <-sig
	assert.Equal(t, item2, 2)
	t.Logf("%s - closing inbox", nanoNow())
	ibox.Close()
	<-ended
}

func nanoNow() string {
	return time.Now().Format(time.RFC3339Nano)
}
