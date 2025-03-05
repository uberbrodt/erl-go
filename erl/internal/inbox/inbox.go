package inbox

import (
	"fmt"
	"iter"
	"sync"
)

type Inbox[M any] struct {
	msgQ   []M
	mx     sync.Mutex
	done   chan struct{}
	closed bool
	cond   *sync.Cond
}

// Create an Inbox that will store messages of type [M].
// An Inbox is written to using the `Enqueue` method, which will
// append itself to the end of a message queue. To read these messages,
// there are four methods:
//
//  1. Calling [Pop], which will return one message or nothing.
//  2. Calling [BlockingPop], which will wait until there is a message available or the inbox is closed.
//  3. A for-range loop with [Iter]
//  4. [Channel], which will return a channel that will receive a message one at a time.
//
// All of the options are roughly equivalent in performance and are safe for concurrent
// use. Options three and four are probably the most useful for concurrent programs.
func New[M any]() *Inbox[M] {
	i := &Inbox[M]{
		msgQ: make([]M, 0, 10),
		done: make(chan struct{}),
	}
	i.cond = sync.NewCond(&i.mx)
	return i
}

// Add a message to the end of the k
func (i *Inbox[M]) Enqueue(msg M) bool {
	i.mx.Lock()
	defer i.mx.Unlock()

	if i.closed {
		return false
	}

	i.msgQ = append(i.msgQ, msg)
	i.cond.Broadcast()

	return true
}

// get and remove a value from the inbox. This is safe to call from multiple go routines.
// if there was no item returned, [ok] returns false
// if the inbox is closed and will never return a value, [closed] will be not nil
func (i *Inbox[M]) Pop() (item M, ok bool, closed error) {
	i.mx.Lock()
	defer i.mx.Unlock()

	if i.closed {
		return item, false, fmt.Errorf("inbox closed")
	}
	if len(i.msgQ) == 0 {
		return item, false, nil
	}

	head := i.msgQ[0]
	i.msgQ = i.msgQ[1:]

	return head, true, nil
}

// Similar to [Pop], but this call will block until it has a value to retrieve.
// This is safe to call from multiple go routines, but keep in mind that [item] may
// be empty in that case, so always ensure that you actually got a value by checking [ok].
//
// If the inbox is closed, [closed] will be non-nil and the caller can expect no more
// messages.
// Under the hood, this is using [sync.Cond] to sleep callers until there are messages.
func (i *Inbox[M]) BlockingPop() (item M, ok bool, closed error) {
	i.mx.Lock()
	defer i.mx.Unlock()

	for len(i.msgQ) == 0 && !i.closed {
		i.cond.Wait()
	}

	if i.closed {
		return item, false, fmt.Errorf("inbox closed")
	}
	if len(i.msgQ) == 0 {
		return item, false, nil
	}

	head := i.msgQ[0]
	i.msgQ = i.msgQ[1:]

	return head, true, nil
}

// Returns a channel that will receive values from the Inbox until it is closed
func (i *Inbox[M]) Channel() <-chan M {
	c := make(chan M)

	go func() {
		defer close(c)

		for {
			item, ok, closed := i.BlockingPop()
			if closed != nil {
				return // Inbox closed, terminate the goroutine
			}
			if !ok {
				continue
			}
			c <- item
		}
	}()
	return c
}

// this is an Iterator function that can be used with a range loop like so:
//
//	for item, ok := range ibox.Iter() {
//		if !ok {
//			log.Println("someone beat us to it, try again")
//			continue
//		}
//		log.Printf("got value: %d", item)
//		sig <- item
//	}
//
// The iterator will exhaust when the inbox is closed. Note that you need to check
// [ok] still to ensure that you actually got a value, since multiple go routines
// may be reading off this same machine.
func (i *Inbox[M]) Iter() iter.Seq2[M, bool] {
	return func(yield func(M, bool) bool) {
		for {
			item, ok, closed := i.BlockingPop()
			if closed != nil {
				return
			}

			if !yield(item, ok) {
				return
			}
		}
	}
}

// Return the number of items in the Inbox
func (i *Inbox[M]) Size() int {
	i.mx.Lock()
	defer i.mx.Unlock()

	return len(i.msgQ)
}

// returns everything in the message queue
func (i *Inbox[M]) Drain() []M {
	i.mx.Lock()
	defer i.mx.Unlock()
	if i.closed {
		return i.msgQ
	}

	result := make([]M, len(i.msgQ))
	copy(result, i.msgQ)
	i.closed = true
	close(i.done)
	i.cond.Broadcast()

	return i.msgQ
}

// closees all iterators and channels associated from this inbox, and prevents
// any messages from being queued/dequeued. [BlockingPop] will also return.
func (i *Inbox[M]) Close() {
	i.mx.Lock()
	defer i.mx.Unlock()
	if i.closed {
		return
	}
	i.closed = true
	// shut down the receiver go routines
	close(i.done)
	i.cond.Broadcast()
}
