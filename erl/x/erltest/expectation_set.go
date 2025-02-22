package erltest

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"sync"
)

type missedMsgs struct {
	msg any
	log *bytes.Buffer
}

type expectationSet struct {
	expected  map[reflect.Type][]*Expectation
	exhausted map[reflect.Type][]*Expectation
	mx        sync.Mutex
	misses    []missedMsgs
}

func NewExpectationSet() *expectationSet {
	return &expectationSet{
		expected:  make(map[reflect.Type][]*Expectation),
		exhausted: make(map[reflect.Type][]*Expectation),
		misses:    []missedMsgs{},
	}
}

func (es *expectationSet) FindMatch(msg any) (*Expectation, error) {
	es.mx.Lock()
	defer es.mx.Unlock()
	msgT := reflect.TypeOf(msg)
	expects := es.expected[msgT]

	errs := new(bytes.Buffer)

	for _, ex := range expects {
		err := ex.Match(msg)
		if err != nil {
			fmt.Fprintf(errs, "%v\n", err)
		} else {
			return ex, nil
		}

	}

	// nothing matched, check if an exhausted call matches
	exhausted, ok := es.exhausted[msgT]
	if ok {
		for _, ex := range exhausted {
			err := ex.Match(msg)
			if err != nil {
				return nil, fmt.Errorf("all expectations for msg '%#v' have been exhausted", msg)
			}
		}
	}

	if len(expects)+len(exhausted) == 0 {
		_, _ = fmt.Fprintf(errs, "there are no expected calls for msg %#v", msg)
	}

	// no exhausted matchers, so we IGNORE this message, but save it as a miss
	es.misses = append(es.misses, missedMsgs{msg: msg, log: errs})
	return nil, errors.New(errs.String())
}

func (es *expectationSet) Add(ex *Expectation) {
	es.mx.Lock()
	defer es.mx.Unlock()

	msgType := ex.msgT
	expects, ok := es.expected[msgType]

	if !ok {
		es.expected[msgType] = []*Expectation{ex}
		es.exhausted[msgType] = []*Expectation{}
	} else {
		es.expected[msgType] = append(expects, ex)
	}
}

func (es *expectationSet) Remove(ex *Expectation) {
	es.mx.Lock()
	defer es.mx.Unlock()

	key := ex.msgT
	expects := es.expected[key]

	for i, expect := range expects {
		if expect == ex {
			es.expected[key] = append(expects[:i], expects[i+1:]...)
			es.exhausted[key] = append(es.exhausted[key], ex)
			break
		}
	}
}

func (es *expectationSet) MustWait() bool {
	es.mx.Lock()
	defer es.mx.Unlock()
	for _, expects := range es.expected {
		for _, e := range expects {
			if e.MustWait() {
				return true
			}
		}
	}
	return false
}

func (es *expectationSet) IsSatisifed() bool {
	es.mx.Lock()
	defer es.mx.Unlock()
	if len(es.expected) == 0 {
		return true
	}

	for _, expects := range es.expected {
		for _, e := range expects {
			if !e.satisfied() {
				return false
			}
		}
	}
	return true
}

// how many expectations, exhausted or pending, are there.
func (es *expectationSet) howMany() int {
	es.mx.Lock()
	defer es.mx.Unlock()
	var cnt int

	for _, exs := range es.expected {
		cnt += len(exs)
	}

	for _, exs := range es.exhausted {
		cnt += len(exs)
	}
	return cnt
}

// Returns all the unsatisifed expectations
func (es *expectationSet) Unsatisifed() map[reflect.Type][]*Expectation {
	es.mx.Lock()
	defer es.mx.Unlock()

	results := make(map[reflect.Type][]*Expectation)

	for t, expects := range es.expected {
		for _, e := range expects {
			if !e.satisfied() {
				exs, ok := results[t]
				if !ok {
					results[t] = []*Expectation{e}
				} else {
					results[t] = append(exs, e)
				}

			}
		}
	}
	return results
}
