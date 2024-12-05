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

func TestSync_CanBeCalledMultipleTimes(t *testing.T) {
	sig := make(chan int)
	ended := make(chan struct{})

	ibox := inbox.New[int]()

	go func() {
		c := ibox.Sync()
		c.L.Lock()
		defer c.L.Unlock()
		t.Log("goroutine start Wait loop")
		for {
			v, ok, err := ibox.Pop()
			if err != nil {
				t.Log("inbox closed")
				// exit go routine
				close(ended)
				return
			}
			if !ok {
				t.Log("no values in inbox, wait for more")
				c.Wait()
				t.Log("Wait() unblocked")
				continue
			}
			t.Logf("got value: %d", v)
			// set value onto our sig channel
			sig <- v
		}
	}()

	t.Log("enqueue item 1")
	ibox.Enqueue(1)
	t.Log("enqueue item 2")
	ibox.Enqueue(2)
	t.Log("close inbox")

	t.Log("waiting for item 1")
	item1 := <-sig
	assert.Equal(t, item1, 1)

	t.Log("waiting for item 2")
	item2 := <-sig
	assert.Equal(t, item2, 2)
	ibox.Close()
	<-ended
}

func TestBlockingPop(t *testing.T) {
	sig := make(chan int)
	ended := make(chan struct{})

	ibox := inbox.New[int]()

	go func() {
		for {
			item, ok, closed := ibox.BlockingPop()
			if closed != nil {
				t.Log("inbox closed")
				close(ended)
				return

			}
			if !ok {
				t.Log("someone beat us to it, try again")
				continue
			}
			t.Logf("got value: %d", item)
			sig <- item
		}
	}()

	t.Log("enqueue item 1")
	ibox.Enqueue(1)
	t.Log("enqueue item 2")
	ibox.Enqueue(2)
	t.Log("close inbox")

	t.Log("waiting for item 1")
	item1 := <-sig
	assert.Equal(t, item1, 1)

	t.Log("waiting for item 2")
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
		for item, ok := range ibox.Iter() {
			if !ok {
				t.Log("someone beat us to it, try again")
				continue
			}
			t.Logf("got value: %d", item)
			sig <- item
		}
		close(ended)
	}()

	t.Log("enqueue item 1")
	ibox.Enqueue(1)
	t.Log("enqueue item 2")
	ibox.Enqueue(2)
	t.Log("close inbox")

	t.Log("waiting for item 1")
	item1 := <-sig
	assert.Equal(t, item1, 1)

	t.Log("waiting for item 2")
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
			t.Logf("got value: %d", item)
			sig <- item
		}
		close(ended)
	}()

	t.Log("enqueue item 1")
	ibox.Enqueue(1)
	t.Log("enqueue item 2")
	ibox.Enqueue(2)
	t.Log("close inbox")

	t.Log("waiting for item 1")
	item1 := <-sig
	assert.Equal(t, item1, 1)

	t.Log("waiting for item 2")
	item2 := <-sig
	assert.Equal(t, item2, 2)
	t.Log("closing inbox")
	ibox.Close()
	<-ended
}
