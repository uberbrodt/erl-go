package inbox

import (
	"sync"
)

type Inbox[M any] struct {
	msgQ   []M
	mx     sync.RWMutex
	done   chan struct{}
	closed bool
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
	return &Inbox[M]{
		msgQ: make([]M, 0, 10),
		done: make(chan struct{}),
	}
}

// Add a message to the end of the k
func (i *Inbox[M]) Enqueue(msg M) bool {
	i.mx.Lock()
	defer i.mx.Unlock()

	if i.closed {
		return false
	}

	i.msgQ = append(i.msgQ, msg)

	return true
}

// Returns a channel that will send values as long as the inbox is open.
// The returned channel will be closed if [Close] is called on the inbox.
func (i *Inbox[M]) Receive() <-chan M {
	rec := make(chan M)

	go func() {
		for {
			v, ok := i.Pop()

			if !ok {
				continue
			}

			select {
			case <-i.done:
				// return if this inbox has been closed
				close(rec)
				return
			case rec <- v:
				// this case will block until the receiver is ready to get the message. We don't
				// need to do anything here.
			}
		}
	}()

	return rec
}

// get and remove a value from the inbox. This is safe to call from multiple go routines.
func (i *Inbox[M]) Pop() (M, bool) {
	var null M
	i.mx.Lock()
	defer i.mx.Unlock()
	if len(i.msgQ) == 0 {
		return null, false
	}

	head := i.msgQ[0]
	tail := i.msgQ[1:]
	i.msgQ = tail

	return head, true
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
