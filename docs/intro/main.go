package main

import (
	"log"
	"time"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
)

type P struct{}

func (p P) Receive(self erl.PID, inbox <-chan any) error {
	for {
		select {
		case _, ok := <-inbox:
			if !ok {
				return nil
			}
		case <-time.After(3 * time.Second):
			log.Printf("The time is: %s\n", time.Now().String())
		}
	}
}

func main() {
	pid := erl.Spawn(P{})
	time.Sleep(10 * time.Second)
	log.Printf("Process is alive?: %t", erl.IsAlive(pid))
	erl.Exit(erl.RootPID(), pid, exitreason.Kill)
	time.Sleep(3 * time.Second)
	log.Printf("Process is alive?: %t", erl.IsAlive(pid))
}
