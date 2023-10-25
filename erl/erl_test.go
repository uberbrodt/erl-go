//go:build !integration

package erl

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/uberbrodt/erl-go/chronos"
	"github.com/uberbrodt/erl-go/erl/exitreason"
)

func TestBiDirectionalChannels(t *testing.T) {
	c := make(chan string)

	writer1 := func() {
		n := 10
		for n > 0 {
			c <- fmt.Sprintf("writer1 %d", n)
			n--
		}
	}

	writer2 := func() {
		n := 10
		for n > 0 {
			c <- fmt.Sprintf("writer2 %d", n)
			n--
		}
	}

	go writer1()
	go writer2()

	loop := true
	for loop {
		select {
		case msg := <-c:
			t.Log(msg)
		case <-time.After(chronos.Dur("1s")):
			t.Log("All done!")
			loop = false
		}
	}
}

func Test_ProcessSpawn(t *testing.T) {
	tr := &TestRunnable{t: t, expected: "foo"}
	var wg sync.WaitGroup
	wg.Add(1)

	p := Spawn(tr)
	sendSignal(p, NewMsg(testRunnableSyncArg{wg: &wg, actual: "foo"}))

	wg.Wait()
}

func Test_Monitors_SendsDown(t *testing.T) {
	doneChan := make(chan int)
	runnableDone := make(chan int)
	var ref Ref
	fn := func(self PID, d chan<- int, msg any) {
		defer func() {
			d <- 1
		}()
		t.Logf("got message in server: %+v", msg)
		// assert.Equal(t, msg.Type, DownMsg)
		down := msg.(DownMsg)
		assert.Equal(t, down.Ref, ref)
		assert.Assert(t, exitreason.IsNormal(down.Reason))
		d <- 0
	}

	monit := func(self PID, runDone chan<- int, inbox <-chan any) error {
		time.Sleep(chronos.Dur("3s"))
		runDone <- 0
		t.Log("Finished runnable")
		return exitreason.Normal
	}

	self := Spawn(&TestSimpleServer{t: t, exec: fn, timeout: chronos.Dur("10s"), done: doneChan})

	_, ref = SpawnMonitor(self, &TestSimpleRunnable{t, monit, runnableDone})

	<-runnableDone
	<-doneChan

	assert.Assert(t, ref != "")
}

func Test_Demonitor_PreventsDownSignal(t *testing.T) {
	doneChan := make(chan int)
	runnableDone := make(chan int)

	r1 := func(ts *TestServer, self PID, inbox <-chan any) error {
		// r1PID = self
		time.Sleep(chronos.Dur("1s"))
		t.Log("Finished runnable")
		runnableDone <- 0
		return exitreason.Normal
	}

	r2 := func(ts *TestServer, self PID, inbox <-chan any) error {
		for {
			select {
			case in := <-inbox:
				switch msg := in.(type) {
				case downSignal:
					t.Errorf("server receieved a down signal, %+v", msg)
					doneChan <- 1
					return exitreason.Exception(fmt.Errorf("server receieved a down signal"))
				}

			case <-time.After(chronos.Dur("5s")):
				doneChan <- 0

			}
		}
	}

	srv := Spawn(&TestServer{t: t, exec: r2})

	task, ref := SpawnMonitor(srv, &TestServer{t: t, exec: r1})
	Demonitor(srv, ref)

	<-runnableDone
	<-doneChan

	assert.Equal(t, IsAlive(task), false)
	assert.Equal(t, IsAlive(srv), true)
}

func Test_Link_SendsExits(t *testing.T) {
	doneChan := make(chan int)
	runnableDone := make(chan int)

	tempFun := func(ts *TestServer, self PID, inbox <-chan any) error {
		time.Sleep(chronos.Dur("3s"))
		t.Log("Finished runnable")
		runnableDone <- 1
		return exitreason.Shutdown("foo")
	}

	srvFn := func(ts *TestServer, self PID, inbox <-chan any) error {
		for {
			select {
			case _, ok := <-inbox:
				if !ok {
					doneChan <- 1
					return exitreason.Normal

				}
			case <-time.After(chronos.Dur("10s")):
				doneChan <- 1

			}
		}
	}

	srv := Spawn(&TestServer{t: t, exec: srvFn, done: doneChan})

	task := SpawnLink(srv, &TestServer{t: t, exec: tempFun})

	<-runnableDone
	<-doneChan

	assert.Equal(t, IsAlive(task), false)
	assert.Equal(t, IsAlive(srv), false)
}

func Test_Link_SendsExitsWhenPanic(t *testing.T) {
	doneChan := make(chan int)
	runnableDone := make(chan int)

	tempFun := func(ts *TestServer, self PID, inbox <-chan any) error {
		time.Sleep(chronos.Dur("3s"))
		t.Log("Finished runnable")
		runnableDone <- 1
		panic("my error")
		// return nil
	}

	srvFn := func(ts *TestServer, self PID, inbox <-chan any) error {
		for {
			select {
			case _, ok := <-inbox:
				if !ok {
					doneChan <- 1
					return exitreason.Normal

				}
			case <-time.After(chronos.Dur("10s")):
				doneChan <- 1

			}
		}
	}

	srv := Spawn(&TestServer{t: t, exec: srvFn, done: doneChan})

	task := SpawnLink(srv, &TestServer{t: t, exec: tempFun})

	<-runnableDone
	<-doneChan

	assert.Equal(t, IsAlive(task), false)
	assert.Equal(t, IsAlive(srv), false)
}

func Test_Unlink_RemovesLink(t *testing.T) {
	doneChan := make(chan int)
	runnableDone := make(chan int)

	r1 := func(ts *TestServer, self PID, inbox <-chan any) error {
		// r1PID = self
		time.Sleep(chronos.Dur("1s"))
		t.Log("Finished runnable")
		runnableDone <- 0
		return exitreason.Normal
	}

	r2 := func(ts *TestServer, self PID, inbox <-chan any) error {
		time.Sleep(chronos.Dur("2s"))
		doneChan <- 0
		for range inbox {
			time.Sleep(chronos.Dur("1s"))
		}
		return exitreason.Normal
	}

	srv := Spawn(&TestServer{t: t, exec: r2})

	task := SpawnLink(srv, &TestServer{t: t, exec: r1})
	Unlink(srv, task)

	<-runnableDone
	<-doneChan

	assert.Equal(t, IsAlive(task), false)
	assert.Equal(t, IsAlive(srv), true)
}
