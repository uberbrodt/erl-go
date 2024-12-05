package inbox

import (
	"fmt"
	"iter"
	"sync"
)

type Inbox[M any] struct {
	msgQ   []M
	mx     sync.RWMutex
	done   chan struct{}
	closed bool
	cond   *sync.Cond
}

// Create an Inbox that will store messages of type [M].
// An Inbox is written to using the `Enqueue` method, which will
// append itself to the end of a message queue. To read these messages,
// there are two methods:
//
//  1. Calling [Pop], which will return one message at a time.
//  2. Call [Receive], which will return a channel that will receive a message one at a time. This
//     method is useful if you expect to call [Pop] in a loop or if you want to have multiple consumers
//     of the inbox and order is not important (a version of the competeing consumers pattern).
func New[M any]() *Inbox[M] {
	i := &Inbox[M]{
		msgQ: make([]M, 0, 10),
		done: make(chan struct{}),
	}
	i.cond = sync.NewCond(&sync.Mutex{})
	return i
}

// Add a message to the end of the k
func (i *Inbox[M]) Enqueue(msg M) bool {
	i.mx.Lock()
	defer i.mx.Unlock()

	if i.closed {
		i.cond.Broadcast()
		return false
	}

	i.msgQ = append(i.msgQ, msg)

	i.cond.Broadcast()
	return true
}

func (i *Inbox[M]) Sync() *sync.Cond {
	// i.mx.Lock()
	// defer i.mx.Unlock()
	return i.cond
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
	tail := i.msgQ[1:]
	i.msgQ = tail

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
	c := i.Sync()
	c.L.Lock()
	defer c.L.Unlock()
	item, ok, closed = i.Pop()
	if closed != nil {
		return
	}
	if !ok {
		c.Wait()
		return i.Pop()
	}
	return
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
// The loop will exit when the inbox is closed. Note that you need to check
// [ok] still to ensure that you actually got a value, since multiple go routines
// may be reading off this same machine.
func (i *Inbox[M]) Iter() iter.Seq2[M, bool] {
	return func(yield func(M, bool) bool) {
		c := i.Sync()
		c.L.Lock()
		defer c.L.Unlock()
		for {
			item, ok, closed := i.Pop()
			if closed != nil {
				return
			}
			if !ok {
				c.Wait()
				item2, ok2, closed := i.Pop()
				if closed != nil {
					return
				}
				if !yield(item2, ok2) {
					return
				}

			}
			if !yield(item, ok) {
				return
			}
		}
	}
}

// Return the number of items in the Inbox
func (i *Inbox[M]) Size() int {
	i.mx.RLock()
	defer i.mx.RUnlock()

	return len(i.msgQ)
}

func (i *Inbox[M]) Close() {
	i.mx.Lock()
	defer i.mx.Unlock()
	// shut down the receiver go routines
	close(i.done)
	i.closed = true
}
