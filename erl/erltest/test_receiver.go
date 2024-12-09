// this package contains the [TestReceiver] which is a process that
// can have message expectations set on them. These expectations match
// messages sent to the process inbox and execute a [TestExpectation] function. The
// function returns true to pass and false to fail the test.
package erltest

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/rs/xid"
	"github.com/uberbrodt/fungo/fun"
	"golang.org/x/exp/maps"
	"gotest.tools/v3/assert"

	"github.com/uberbrodt/erl-go/chronos"
	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/exitreason"
	"github.com/uberbrodt/erl-go/erl/exitwaiter"
	"github.com/uberbrodt/erl-go/erl/genserver"
)

var (
	// DefaultReceiverTimeout time.Duration = chronos.Dur("9m59s")
	DefaultReceiverTimeout time.Duration = chronos.Dur("9m59s")
	DefaultWaitTimeout     time.Duration = chronos.Dur("5s")
)

type ReceiverOpt func(ro receiverOptions) receiverOptions

// used to inject more complex mocks based on a TestReceiver and have them checked
// when [Wait] is called
type TestDependency interface {
	Pass() (int, bool)
}

type receiverOptions struct {
	timeout     time.Duration
	waitTimeout time.Duration
	waitExit    time.Duration
	noFail      bool
	name        string
	logger      *slog.Logger
	parent      erl.PID
}

// Specify how long the test receiver should run for before stopping.
// this needs to be set otherwise tests will hang until exceptions are matched or
// the 10min Go default is reached. See [DefaultReceiverTimeout]
// If the '-timeout' option is greater or less than the [DefaultReceiverTimeout], then it
// will be used instead. With that set, you shouldn't need to set this option unless
// you explicitly want to end before your test timeout, such as debugging a broken test
// or negative testing expect options.
func ReceiverTimeout(t time.Duration) ReceiverOpt {
	return func(ro receiverOptions) receiverOptions {
		ro.timeout = t
		ro.waitExit = t - time.Second
		return ro
	}
}

// Specify the minimum amount of time that [TestReciever.Wait] will execute for
// before expectations like [expect.Times] or [expect.AtMost] will pass.
// Wait will finish before this timeout if only options like [expect.AtMost] are used.
// See [DefaultWaitTimeout] for the default. func WaitTimeout(t time.Duration) ReceiverOpt {
func WaitTimeout(t time.Duration) ReceiverOpt {
	return func(ro receiverOptions) receiverOptions {
		ro.waitTimeout = t
		return ro
	}
}

// Set this if you do not want the TestReceiver to call [testing.T.Fail] in
// the [Wait] method.
func NoFail() ReceiverOpt {
	return func(ro receiverOptions) receiverOptions {
		ro.noFail = true
		return ro
	}
}

// Set a name for the test receiver, that will be used in log messages
func Name(name string) ReceiverOpt {
	return func(ro receiverOptions) receiverOptions {
		ro.name = name
		return ro
	}
}

// Set a parent for this test process. Defaults to [erl.rootPID]
func Parent(parent erl.PID) ReceiverOpt {
	return func(ro receiverOptions) receiverOptions {
		ro.parent = parent
		return ro
	}
}

// XXX: might remove.
func SetLogger(logger *slog.Logger) ReceiverOpt {
	return func(ro receiverOptions) receiverOptions {
		ro.logger = logger
		return ro
	}
}

// Creates a new TestReceiver, which is a process that you can set
// message matching expectations on.
func NewReceiver(t *testing.T, opts ...ReceiverOpt) (erl.PID, *TestReceiver) {
	rOpts := receiverOptions{
		timeout:     DefaultReceiverTimeout,
		waitExit:    DefaultReceiverTimeout - time.Second,
		waitTimeout: DefaultWaitTimeout,
		name:        fmt.Sprintf("%s-test-receiver", xid.New().String()),
		parent:      erl.RootPID(),
	}

	if tout, ok := t.Deadline(); ok {
		testExit := time.Until(tout) - time.Second
		if testExit > DefaultReceiverTimeout {
			fmt.Printf("-timeout greater than DefaultReceiverTimeout, adjusting: %s", testExit)
			rOpts.timeout = testExit
			rOpts.waitExit = testExit - time.Second
		}

		if testExit < DefaultReceiverTimeout {
			fmt.Printf("-timeout less than DefaultReceiverTimeout, adjusting: %s", testExit)
			rOpts.timeout = testExit
			rOpts.waitExit = testExit - time.Second
		}
	}

	for _, o := range opts {
		rOpts = o(rOpts)
	}
	expectations := make(map[reflect.Type]Expectation)
	castExpects := make(map[reflect.Type]Expectation)
	callExpects := make(map[reflect.Type]Expectation)
	allExpects := make(map[string]Expectation)
	testDeps := make([]TestDependency, 0)
	deps := make([]erl.PID, 0)

	tr := &TestReceiver{
		t: t, msgExpects: expectations, castExpects: castExpects, callExpects: callExpects,
		opts: rOpts, noFail: rOpts.noFail, allExpects: allExpects, testdeps: testDeps, deps: deps,
	}
	if rOpts.logger != nil {
		tr.log = rOpts.logger
	} else {
		tr.log = slog.New(slog.NewTextHandler(os.Stderr, nil)).With("erltest.test-receiver", rOpts.name)
	}
	pid := erl.Spawn(tr)
	tr.setSelf(pid)
	t.Logf("TestReceiver PID spawned: %+v", pid)

	erl.ProcessFlag(pid, erl.TrapExit, true)
	t.Cleanup(func() {
		tr.log.Info("executing TestReceiver cleanup...")
		tr.Stop()
	})
	return pid, tr
}

type TestReceiver struct {
	t           *testing.T
	msgExpects  map[reflect.Type]Expectation
	castExpects map[reflect.Type]Expectation
	callExpects map[reflect.Type]Expectation
	allExpects  map[string]Expectation
	failures    []*ExpectationFailure
	testdeps    []TestDependency
	deps        []erl.PID
	msgCnt      int
	self        erl.PID
	// if set to true, t.FailNow will not be called in [Pass] or [Wait]
	noFail    bool
	mx        sync.RWMutex
	selfmx    sync.RWMutex
	testEnded bool
	opts      receiverOptions
	exiting   bool
	log       *slog.Logger
}

func (tr *TestReceiver) getExiting() bool {
	defer tr.selfmx.RUnlock()
	tr.selfmx.RLock()
	return tr.exiting
}

func (tr *TestReceiver) setExiting(status bool) {
	defer tr.selfmx.Unlock()
	tr.selfmx.Lock()
	tr.exiting = status
}

func (tr *TestReceiver) getTestEnded() bool {
	defer tr.selfmx.RUnlock()
	tr.selfmx.RLock()
	return tr.testEnded
}

func (tr *TestReceiver) setTestEnded(status bool) {
	defer tr.selfmx.Unlock()
	tr.selfmx.Lock()
	tr.testEnded = status
}

func (tr *TestReceiver) getSelf() erl.PID {
	defer tr.selfmx.RUnlock()
	tr.selfmx.RLock()
	return tr.self
}

func (tr *TestReceiver) Self() erl.PID {
	defer tr.selfmx.RUnlock()
	tr.selfmx.RLock()
	return tr.self
}

func (tr *TestReceiver) setSelf(pid erl.PID) {
	defer tr.selfmx.Unlock()
	tr.selfmx.Lock()
	tr.self = pid
}

// only log if the test isn't existing or ended.
func (tr *TestReceiver) safeTLogf(format string, args ...any) {
	args = append([]any{"pid", tr.getSelf()}, args...)
	tr.log.Info(format, args...)
}

// log and mark the test as failed
func (tr *TestReceiver) safeTError(format string, args ...any) {
	prefix := fmt.Sprintf("[TestReceiver: %s|PID: %+v]: %s", tr.opts.name, tr.getSelf(), format)
	tr.t.Errorf(prefix, args...)
}

func (tr *TestReceiver) Receive(self erl.PID, inbox <-chan any) error {
	for {
		select {
		case msg, ok := <-inbox:
			if !ok {
				return exitreason.Normal
			}
			switch v := msg.(type) {
			case erl.ExitMsg:
				tr.setExiting(true)
				if errors.Is(v.Reason, exitreason.TestExit) {
					tr.log.Info("received a TestExit, shutting down", "reason", v.Reason, "sending-proc", v.Proc)
					return exitreason.Normal
				}
			}
			tr.safeTLogf("message received", "msg", msg)

			tr.mx.Lock()
			tr.check(msg)
			tr.mx.Unlock()
		case <-time.After(tr.opts.timeout):
			tr.safeTError("receive timeout")
			// end test and call finish to print the expectations that didn't pass
			tr.setTestEnded(true)
			tr.finish()

			return exitreason.Timeout
		}
	}
}

func (tr *TestReceiver) check(msg any) {
	tr.msgCnt = tr.msgCnt + 1

	switch v := msg.(type) {
	case genserver.CastRequest:
		castMsgT := reflect.TypeOf(v.Msg)
		for match, ex := range tr.castExpects {
			if castMsgT == match {
				// pass in the unwrapped message
				fail := tr.checkMatch(match, v.Msg, nil, ex)
				if fail != nil {
					tr.failures = append(tr.failures, fail)
				}
			}
		}
	case genserver.CallRequest:
		callMsgT := reflect.TypeOf(v.Msg)
		for match, ex := range tr.callExpects {
			if callMsgT == match {
				fail := tr.checkMatch(match, v.Msg, &v.From, ex)
				if fail != nil {
					tr.failures = append(tr.failures, fail)
				}
			}
		}
	default:
		msgT := reflect.TypeOf(msg)
		for match, ex := range tr.msgExpects {
			if msgT == match {
				fail := tr.checkMatch(match, msg, nil, ex)
				if fail != nil {
					tr.failures = append(tr.failures, fail)
				}
			}
		}

	}
}

func (tr *TestReceiver) checkMatch(match reflect.Type, msg any, from *genserver.From, ex Expectation) (failure *ExpectationFailure) {
	arg := ExpectArg{Match: match, Exp: ex, Msg: msg, Self: tr.getSelf(), MsgCount: tr.msgCnt, From: from}
	if nextEx, fail := ex.Check(arg); fail != nil {
		return fail
	} else if nextEx != nil {
		return tr.checkMatch(match, msg, from, nextEx)
	} else {
		return nil
	}
}

// Register an expectation with this TestReciever. It will be checked
// when Pass is called (and as a consequnce, cause [Wait] to block until its success)
func (tr *TestReceiver) WaitOn(e ...Expectation) {
	for _, ex := range e {
		tr.allExpects[ex.ID()] = ex
	}
}

// TODO: implement
// func (tr *TestReceiver) Join(pid erl.PID, td TestDependency) {
// 	// if the pid is not nil, we should link to this process so that it will exit when
// 	// this test receiver exits.
// 	if !pid.IsNil() {
// 		erl.Link(tr.getSelf(), pid)
// 	}
//
// 	tr.testdeps = append(tr.testdeps, td)
// }

// starts the process via [startLink] with the TestReceiver as the parent. If [startLink] returns
// an error the test is failed. The process will be synchronously killed when calling [Stop]
func (tr *TestReceiver) StartSupervised(startLink func(self erl.PID) (erl.PID, error)) erl.PID {
	tr.t.Helper()
	pid, err := startLink(tr.getSelf())

	assert.NilError(tr.t, err)

	tr.deps = append(tr.deps, pid)

	return pid
}

// Set an expectation that will be matched whenever a [matchTerm] msg type is received.
func (tr *TestReceiver) Expect(matchTerm any, e Expectation) {
	t := reflect.TypeOf(matchTerm)
	tr.allExpects[e.ID()] = e
	tr.msgExpects[t] = e
}

// This is like [Expect] but is only tested against [genserver.CastRequest] messages.
func (tr *TestReceiver) ExpectCast(matchTerm any, e Expectation) {
	t := reflect.TypeOf(matchTerm)
	tr.allExpects[e.ID()] = e
	tr.castExpects[t] = e
}

// This is like [Expect] but is only tested against [genserver.CallRequest] messages.
// NOTE: You should use [genserver.Reply] to send a response to the [genserver.From], otherwise
// the caller will timeout
func (tr *TestReceiver) ExpectCall(matchTerm any, e Expectation) {
	t := reflect.TypeOf(matchTerm)
	tr.allExpects[e.ID()] = e
	tr.callExpects[t] = e
}

// returns the number of failed expectations and whether
// all expectations have been satisifed. An expectation is
// not satisfied unless it is invoked in the correct time and order
func (tr *TestReceiver) Pass() (int, bool) {
	tr.mx.RLock()
	defer tr.mx.RUnlock()
	expects := maps.Values(tr.allExpects)
	reducer := func(list []Expectation) bool {
		return fun.Reduce(list, true, func(v Expectation, acc bool) bool {
			if !acc {
				return acc
			}

			ok := v.Satisfied(tr.getTestEnded())
			return ok
		})
	}

	return len(tr.failures), reducer(expects)
}

func (tr *TestReceiver) finish() (done bool, failed bool) {
	fails, passed := tr.Pass()
	if fails > 0 {
		tr.mx.Lock()
		defer tr.mx.Unlock()
		tr.safeTLogf("expectation failures", "count", fails)
		for i, f := range tr.failures {
			tr.safeTLogf("expectation failure", "failure_cnt",
				i, "name", f.Exp.Name(),
				"reason", f.Reason,
				"match", f.Match,
				"msg", f.Msg)
		}
		if !tr.noFail {
			tr.t.Fail()
		} else {
			return false, true
		}
	}
	return passed, false
}

// Returns when the [WaitTimeout] expires or an expectation fails. If at the end of the timeout not
// all expectations are satisifed, the test is failed. Call this after you have sent your messages
// and want fail if your expecations don't pass
func (tr *TestReceiver) Wait() {
	now := time.Now()
	// we use a local variable here so we don't increase lock contentention calling
	// tr.setTestEnded every iteration
	var testEnded bool
	for {

		if time.Since(now) > tr.opts.waitExit {
			tr.log.Info("test timed out waiting for expectations to be fulfilled")
			tr.finish()
			return
		}

		if time.Since(now) > tr.opts.waitTimeout {
			if !testEnded {
				tr.setTestEnded(true)
				testEnded = true
			}
			_, passed := tr.Pass()
			if passed {
				tr.finish()
				return
			}
			continue
		}

		failures, passed := tr.Pass()
		//  all passed, return so test ends
		if passed {
			tr.finish()
			return
		}

		// we had a failure before the waitTimeout, so exit
		if failures > 0 {
			tr.finish()
			return
		}
		time.Sleep(time.Millisecond)
	}
}

// will cause the test receiver to exit, sending signals to linked and monitoring
// processes. This is not needed for normal test cleanup (that is handled via [t.Cleanup()])
func (tr *TestReceiver) Stop() {
	// first, stop dependencies
	if !erl.IsAlive(tr.getSelf()) {
		return
	}

	var depWG sync.WaitGroup
	if len(tr.deps) > 0 {
		depWG.Add(len(tr.deps))

		for _, dep := range tr.deps {
			_, err := exitwaiter.New(tr.t, tr.getSelf(), dep, &depWG)
			if err != nil {
				tr.t.Errorf("FAILURE: testreceiver %s could not start exitwaiter for dep %+v: %+v", tr.getSelf(), dep, err)
				return
			}
			erl.Exit(tr.opts.parent, dep, exitreason.Kill)

		}

		tr.t.Logf("test receiver %+v waiting for deps to stop", tr.getSelf())
		depWG.Wait()
		tr.t.Logf("test receiver %+v deps stopped", tr.getSelf())
	}

	if !erl.IsAlive(tr.getSelf()) {
		return
	}
	var wg sync.WaitGroup
	wg.Add(1)
	_, err := exitwaiter.New(tr.t, tr.opts.parent, tr.getSelf(), &wg)
	if err != nil {
		tr.t.Errorf("FAILURE starting exitwaiter for test process: %+v", err)
		return
	}
	erl.Exit(tr.opts.parent, tr.getSelf(), exitreason.TestExit)
	wg.Wait()
	tr.t.Logf("test receiver has stopped: %s ", tr.getSelf())
}

func (tr *TestReceiver) Failures() []*ExpectationFailure {
	return tr.failures
}

// return the [*testing.T]. Don't use this in situations that could run after a
// test is failed, the race detector doesn't like that.
func (tr *TestReceiver) T() *testing.T {
	return tr.t
}
