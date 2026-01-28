// this package contains the [TestReceiver] which is a process that
// can have message expectations set on them. These expectations match
// messages sent to the process inbox and execute a [TestExpectation] function. The
// function returns true to pass and false to fail the test.
package erltest

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"runtime"
	"sync"
	"time"

	"github.com/rs/xid"

	"github.com/uberbrodt/erl-go/chronos"
	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/erltest"
	"github.com/uberbrodt/erl-go/erl/exitreason"
	"github.com/uberbrodt/erl-go/erl/exitwaiter"
	"github.com/uberbrodt/erl-go/erl/genserver"
)

var (
	// a signal value to indicate that the CallExpect should not reply to the caller
	NoCallReply any = "\x07"
	// DefaultReceiverTimeout time.Duration = chronos.Dur("9m59s")
	DefaultReceiverTimeout time.Duration = chronos.Dur("9m59s")
	DefaultWaitTimeout     time.Duration = chronos.Dur("5s")
)

// Making an interface for [testing.T] so that we can test the TestReceiver
//
//go:generate mockgen  -destination ./internal/mock/tlike.go -package mock . TLike
type TLike interface {
	Errorf(format string, args ...any)
	Logf(format string, args ...any)
	Failed() bool
	Fatalf(format string, args ...any)
	Log(args ...any)
	Helper()
	FailNow()
	Deadline() (time.Time, bool)
	Cleanup(func())
	Error(args ...any)
}

type callExpectation struct {
	e     Expectation
	reply any
}

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

// Create [receiverOptions] from the deprecated [erltest] package
// this is a transitional method and will be removed when this becomes
// the standard testing package.
func OptFromLegacy(optFns ...erltest.ReceiverOpt) ReceiverOpt {
	return func(ro receiverOptions) receiverOptions {
		opts := erltest.NewOpts(optFns...)
		ro.timeout = opts.Timeout
		ro.waitTimeout = opts.Timeout
		ro.waitExit = opts.WaitExit
		ro.noFail = opts.NoFail
		ro.name = opts.Name
		ro.logger = opts.Logger
		ro.parent = opts.Parent
		return ro
	}
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
func NewReceiver(t TLike, opts ...ReceiverOpt) (erl.PID, *TestReceiver) {
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
			fmt.Printf("-timeout greater than DefaultReceiverTimeout, adjusting: %s\n", testExit)
			rOpts.timeout = testExit
			rOpts.waitExit = testExit - time.Second
		}

		if testExit < DefaultReceiverTimeout {
			fmt.Printf("-timeout less than DefaultReceiverTimeout, adjusting: %s\n", testExit)
			rOpts.timeout = testExit
			rOpts.waitExit = testExit - time.Second
		}
	}

	for _, o := range opts {
		rOpts = o(rOpts)
	}

	expectationSet := NewExpectationSet()
	testDeps := make([]TestDependency, 0)
	deps := make([]erl.PID, 0)
	testf := make([]*ExpectationFailure, 0)

	tr := &TestReceiver{
		t: t, opts: rOpts, noFail: rOpts.noFail,
		testdeps:     testDeps,
		deps:         deps,
		expectations: expectationSet,
		_failures:    testf,
	}
	if rOpts.logger != nil {
		tr.log = rOpts.logger
	} else {
		tr.log = slog.New(slog.NewTextHandler(os.Stderr, nil)).With("erltest.test-receiver", rOpts.name)
	}
	pid := erl.Spawn(tr)
	tr.setSelf(pid)
	t.Logf("%s - [%v] spawned\n", time.Now().Format(time.RFC3339Nano), tr)

	erl.ProcessFlag(pid, erl.TrapExit, true)
	t.Cleanup(func() {
		tr.log.Info("executing TestReceiver cleanup...")
		tr.Stop()
	})
	return pid, tr
}

type TestReceiver struct {
	t  TLike
	id string
	// msgExpects map[reflect.Type][]Expectation
	expectations *expectationSet
	// allExpects map[string]Expectation
	_failures []*ExpectationFailure
	testdeps  []TestDependency
	deps      []erl.PID
	msgCnt    int
	self      erl.PID
	// if set to true, t.FailNow will not be called in [Pass] or [Wait]
	noFail bool
	mx     sync.RWMutex
	// selfmx     sync.RWMutex
	failuresMx sync.RWMutex
	// TODO: rename to waitExpired
	waitExpired bool
	opts        receiverOptions
	exiting     bool
	log         *slog.Logger
	openSig     chan struct{}
}

func (tr *TestReceiver) getExiting() bool {
	defer tr.mx.RUnlock()
	tr.mx.RLock()
	return tr.exiting
}

func (tr *TestReceiver) setExiting(status bool) {
	defer tr.mx.Unlock()
	tr.mx.Lock()
	tr.exiting = status
}

func (tr *TestReceiver) getWaitExpired() bool {
	defer tr.mx.RUnlock()
	tr.mx.RLock()
	return tr.waitExpired
}

func (tr *TestReceiver) setWaitExpired(status bool) {
	defer tr.mx.Unlock()
	tr.mx.Lock()
	tr.waitExpired = status
}

func (tr *TestReceiver) getSelf() erl.PID {
	defer tr.mx.RUnlock()
	tr.mx.RLock()
	return tr.self
}

func (tr *TestReceiver) Self() erl.PID {
	defer tr.mx.RUnlock()
	tr.mx.RLock()
	return tr.self
}

func (tr *TestReceiver) setSelf(pid erl.PID) {
	defer tr.mx.Unlock()
	tr.mx.Lock()
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
			// tr.safeTLogf("message received", "msg", msg)

			tr.mx.Lock()
			tr.check(msg)
			tr.mx.Unlock()
		case <-time.After(tr.opts.timeout):
			tr.safeTError("receive timeout")
			// end test and call finish to print the expectations that didn't pass
			tr.setWaitExpired(true)
			tr.timeout()

			return exitreason.Timeout
		}
	}
}

func (tr *TestReceiver) check(msg any) {
	tr.msgCnt = tr.msgCnt + 1

	var match *Expectation
	var err error
	var callReq *genserver.CallRequest
	switch v := msg.(type) {
	case genserver.CastRequest:
		match, err = tr.expectations.FindMatch(v.Msg)
		if err != nil {
			e := fmt.Errorf("expectation failed: %v", err)
			tr.appendFailure(&ExpectationFailure{Match: v, Exp: match, Msg: v.Msg, Reason: e})
			tr.t.Error(e)

			return
		}

	case genserver.CallRequest:
		callReq = &v
		match, err = tr.expectations.FindMatch(v.Msg)
		if err != nil {
			tr.t.Errorf("expectation failed: %v", err)
			return
		}

	default:
		match, err = tr.expectations.FindMatch(v)
		if err != nil {
			e := fmt.Errorf("expectation failed: %v", err)
			tr.appendFailure(&ExpectationFailure{Match: v, Exp: match, Msg: v, Reason: e})
			tr.t.Error(e)
			return
		}

	}

	// there was no matching expectation, return.
	if match == nil {
		return
	}
	// increments the counter and returns an action we should perform
	do := match.call()

	if callReq != nil {
		if match.reply != NoCallReply {
			genserver.Reply(callReq.From, match.reply)
		}
		if do != nil {
			do(ExpectArg{Msg: callReq.Msg, From: &callReq.From, Self: tr.self, Exp: match})
		}
	} else {
		if do != nil {
			switch v := msg.(type) {
			case genserver.CastRequest:
				do(ExpectArg{Msg: v.Msg, Self: tr.self, Exp: match})
			default:
				do(ExpectArg{Msg: v, Self: tr.self, Exp: match})
			}
		}
	}

	// Two things happen here:
	// * the matching call no longer needs to check prerequisite calls,
	// * and the prerequisite calls are no longer expected, so remove them.
	preReqCalls := match.dropPrereqs()
	for _, preReqCall := range preReqCalls {
		tr.expectations.Remove(preReqCall)
	}

	if match.exhausted() {
		tr.expectations.Remove(match)
	}
}

// starts the process via [startLink] with the TestReceiver as the parent. If [startLink] returns
// an error the test is failed. The process will be synchronously killed when calling [Stop]
func (tr *TestReceiver) StartSupervised(startLink func(self erl.PID) (erl.PID, error)) erl.PID {
	tr.t.Helper()
	pid, err := startLink(tr.getSelf())
	if err != nil {
		tr.t.Fatalf("failed starting supervised process: %v", err)
	}

	tr.deps = append(tr.deps, pid)

	return pid
}

// Set an expectation that will be matched whenever a [matchTerm] msg type is received.
func (tr *TestReceiver) Expect(matchTerm any, m Matcher) *Expectation {
	e := newExpect(tr.t, m, reflect.TypeOf(matchTerm))
	tr.expectations.Add(e)
	return e
}

// This is like [Expect] but is only tested against [genserver.CastRequest] messages.
func (tr *TestReceiver) ExpectCast(matchTerm any, m Matcher) *Expectation {
	e := newExpect(tr.t, m, reflect.TypeOf(matchTerm))
	tr.expectations.Add(e)
	return e
}

// Sets an expectation about a [genserver.Call] for this TestReceiver. The [reply]
// is the value that will be returned to the caller when a [matchTerm] msg is received.
//
// If you want to *not* send a reply (say you're testing Call timeouts), then set [reply]
// to the signal value [NoCallReply].
func (tr *TestReceiver) ExpectCall(matchTerm any, m Matcher, reply any) *Expectation {
	e := newExpect(tr.t, m, reflect.TypeOf(matchTerm))
	e.reply = reply
	tr.expectations.Add(e)
	return e
}

func (tr *TestReceiver) timeout() {
	tr.t.Errorf("%s - [%v] timed out waiting for expectation to be fulfilled\n", time.Now().Format(time.RFC3339Nano), tr)
	tr.printUnsatisifed()
	tr.printUnmatchedMsgs()
}

func (tr *TestReceiver) printUnmatchedMsgs() {
	buf := new(bytes.Buffer)
	fmt.Fprintln(buf, "UNMATCHED MSGS: [")
	for _, missed := range tr.expectations.misses {
		fmt.Fprintf(buf, "%+v\n", missed.msg)
	}
	fmt.Fprintln(buf, "]")

	tr.t.Log(buf)
}

func (tr *TestReceiver) printUnsatisifed() {
	buf := new(bytes.Buffer)
	exs1 := tr.expectations.Unsatisifed()

	fmt.Fprintln(buf, "UNSATISIFED EXPECTATIONS:[")
	for msgT, casts := range exs1 {
		fmt.Fprintf(buf, "%v\n", msgT)
		for _, c := range casts {
			tr.appendFailure(&ExpectationFailure{Exp: c, Match: msgT, Reason: fmt.Errorf("unsatisifed expectation: %v", c)})
			fmt.Fprintf(buf, "%v\n", c)
		}
	}
	fmt.Fprintln(buf, "]")

	tr.t.Log(buf)
}

// Returns when the [ReceiverTimeout] expires or an expectation fails.
// It will not return *before* the [WaitTimeout]; this gives us a minimum amount of time for
// expectations to match messages.
//
// Call this after you have sent your messages
func (tr *TestReceiver) Wait() {
	now := time.Now()
	tr.t.Logf("%s - [%v]] waiting for expectations to be fulfilled", now.Format(time.RFC3339Nano), tr)

	if tr.expectations.howMany() == 0 {
		tr.t.Logf("%s - [%v] no expectations, returning from Wait()", time.Now().Format(time.RFC3339Nano), tr)
		return
	}
	// we use a local variable here so we don't increase lock contentention calling
	// tr.setTestEnded every iteration
	if tr.t.Failed() {
		tr.t.Logf("%s - [%v] test failed, returning from Wait()", time.Now().Format(time.RFC3339Nano), tr)
		return
		tr.printUnsatisifed()
		tr.printUnmatchedMsgs()
		return
	}

	if tr.expectations.MustWait() {
		for {
			// if the test has failed, exit
			if tr.t.Failed() {
				tr.t.Logf("%s - [%v] test failed, returning from Wait()", time.Now().Format(time.RFC3339Nano), tr)
				tr.printUnsatisifed()
				tr.printUnmatchedMsgs()
				return
			}
			if time.Since(now) >= tr.opts.waitTimeout {
				tr.t.Logf("%s - [%v] wait timeout expired", time.Now().Format(time.RFC3339Nano), tr)
				break
			}
			runtime.Gosched()
			time.Sleep(10 * time.Millisecond)
		}
	}

	for {
		if time.Since(now) > tr.opts.waitExit {
			tr.t.Logf("%s - [%v] test timed out in Wait()\n", time.Now().Format(time.RFC3339Nano), tr)
			tr.timeout()
			return
		}

		if tr.t.Failed() {
			tr.t.Logf("%s - [%v] test failed, returning from Wait()\n", time.Now().Format(time.RFC3339Nano), tr)
			tr.printUnsatisifed()
			tr.printUnmatchedMsgs()
			return
		}

		if tr.expectations.IsSatisifed() {
			tr.t.Logf("%s - [%v] expectations all satisfied, returning from Wait()\n", time.Now().Format(time.RFC3339Nano), tr)
			return
		}

		runtime.Gosched()
		time.Sleep(10 * time.Millisecond)
	}
}

// WaitOnChannel blocks until the provided channel is closed or receives a value,
// the test fails, or the receiver timeout is reached.
//
// # Why use WaitOnChannel?
//
// The standard [Wait] method blocks until all [Expectation] instances are satisfied
// based on their matcher and call count constraints (e.g., [Times], [MinTimes]).
// However, when using [AnyTimes] expectations, there is no "satisfied" state to
// wait for—they are always considered satisfied since their minimum call count is 0.
//
// In these cases, test completion must be signaled through side effects rather than
// expectation satisfaction. WaitOnChannel allows you to use a channel closed from
// within a [Do] callback to signal when the test should proceed.
//
// # Example
//
//	done := make(chan struct{})
//	tr.Expect(initMsg{}, gomock.Any()).AnyTimes().Do(func(arg ExpectArg) {
//	    msg := arg.Msg.(initMsg)
//	    if msg.id == "child3" {
//	        close(done) // signal that we've seen the message we care about
//	    }
//	})
//
//	// trigger the action that sends messages...
//	erl.Exit(trPID, childPID, exitreason.Kill)
//
//	tr.WaitOnChannel(done) // blocks until done is closed
//
// See also: [WaitOnFunc], [WaitOnWaitGroup]
func (tr *TestReceiver) WaitOnChannel(ch <-chan struct{}) {
	now := time.Now()
	tr.t.Logf("%s - [%v] WaitOnChannel: waiting for channel signal", now.Format(time.RFC3339Nano), tr)

	for {
		select {
		case <-ch:
			tr.t.Logf("%s - [%v] WaitOnChannel: channel signaled, returning", time.Now().Format(time.RFC3339Nano), tr)
			return
		default:
			if time.Since(now) > tr.opts.waitExit {
				tr.t.Errorf("%s - [%v] WaitOnChannel: timed out waiting for channel signal", time.Now().Format(time.RFC3339Nano), tr)
				return
			}

			if tr.t.Failed() {
				tr.t.Logf("%s - [%v] WaitOnChannel: test failed, returning", time.Now().Format(time.RFC3339Nano), tr)
				return
			}

			runtime.Gosched()
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// WaitOnFunc blocks until the provided function returns true, the test fails,
// or the receiver timeout is reached. The function is polled every 10ms.
//
// This is useful when test completion depends on external state that can be
// queried, rather than messages received by the TestReceiver.
//
// See [WaitOnChannel] for a detailed explanation of why these methods exist.
//
// # Example
//
//	tr.Expect(initMsg{}, gomock.Any()).AnyTimes()
//
//	// trigger the action that eventually registers a process...
//	erl.Exit(trPID, childPID, exitreason.Kill)
//
//	tr.WaitOnFunc(func() bool {
//	    _, ok := erl.WhereIs("child1-name")
//	    return ok // returns true when child1 has been restarted and registered
//	})
//
// See also: [WaitOnChannel], [WaitOnWaitGroup]
func (tr *TestReceiver) WaitOnFunc(fn func() bool) {
	now := time.Now()
	tr.t.Logf("%s - [%v] WaitOnFunc: waiting for function to return true", now.Format(time.RFC3339Nano), tr)

	for {
		if fn() {
			tr.t.Logf("%s - [%v] WaitOnFunc: function returned true, returning", time.Now().Format(time.RFC3339Nano), tr)
			return
		}

		if time.Since(now) > tr.opts.waitExit {
			tr.t.Errorf("%s - [%v] WaitOnFunc: timed out waiting for function to return true", time.Now().Format(time.RFC3339Nano), tr)
			return
		}

		if tr.t.Failed() {
			tr.t.Logf("%s - [%v] WaitOnFunc: test failed, returning", time.Now().Format(time.RFC3339Nano), tr)
			return
		}

		runtime.Gosched()
		time.Sleep(10 * time.Millisecond)
	}
}

// WaitOnWaitGroup blocks until the provided [sync.WaitGroup] counter reaches zero,
// the test fails, or the receiver timeout is reached.
//
// This is useful when multiple independent signals need to be coordinated before
// the test can proceed.
//
// See [WaitOnChannel] for a detailed explanation of why these methods exist.
//
// # Example
//
//	var wg sync.WaitGroup
//	wg.Add(2)
//
//	tr.Expect(initMsg{}, gomock.Any()).AnyTimes().Do(func(arg ExpectArg) {
//	    msg := arg.Msg.(initMsg)
//	    if msg.id == "child1" || msg.id == "child2" {
//	        wg.Done()
//	    }
//	})
//
//	// trigger the action...
//	erl.Exit(trPID, childPID, exitreason.Kill)
//
//	tr.WaitOnWaitGroup(&wg) // blocks until both child1 and child2 have initialized
//
// See also: [WaitOnChannel], [WaitOnFunc]
func (tr *TestReceiver) WaitOnWaitGroup(wg *sync.WaitGroup) {
	now := time.Now()
	tr.t.Logf("%s - [%v] WaitOnWaitGroup: waiting for WaitGroup", now.Format(time.RFC3339Nano), tr)

	// Use a channel to convert WaitGroup.Wait() into a selectable operation
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	for {
		select {
		case <-done:
			tr.t.Logf("%s - [%v] WaitOnWaitGroup: WaitGroup done, returning", time.Now().Format(time.RFC3339Nano), tr)
			return
		default:
			if time.Since(now) > tr.opts.waitExit {
				tr.t.Errorf("%s - [%v] WaitOnWaitGroup: timed out waiting for WaitGroup", time.Now().Format(time.RFC3339Nano), tr)
				return
			}

			if tr.t.Failed() {
				tr.t.Logf("%s - [%v] WaitOnWaitGroup: test failed, returning", time.Now().Format(time.RFC3339Nano), tr)
				return
			}

			runtime.Gosched()
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// will cause the test receiver to exit, sending signals to linked and monitoring
// processes. This is not needed for normal test cleanup (that is handled via [t.Cleanup()])
func (tr *TestReceiver) Stop() {
	tr.t.Logf("%s - [%v] stopping...\n", time.Now().Format(time.RFC3339Nano), tr)
	if !erl.IsAlive(tr.getSelf()) {
		tr.t.Logf("%s - [%v] testreceiver dead, returning from Stop()\n", time.Now().Format(time.RFC3339Nano), tr)
		return
	}

	var depWG sync.WaitGroup
	// first, stop dependencies
	if len(tr.deps) > 0 {
		depWG.Add(len(tr.deps))

		for _, dep := range tr.deps {
			_, err := exitwaiter.New(tr.t, tr.getSelf(), dep, &depWG)
			if err != nil {
				tr.t.Errorf("FAILURE: testreceiver %s could not start exitwaiter for dep %+v: %+v\n", tr.getSelf(), dep, err)
				return
			}
			erl.Exit(tr.opts.parent, dep, exitreason.Kill)

		}

		tr.t.Logf("%s - [%v] waiting for deps to stop\n", time.Now().Format(time.RFC3339Nano), tr)
		depWG.Wait()
		tr.t.Logf("%s - [%v] deps stopped\n", time.Now().Format(time.RFC3339Nano), tr)
	}

	if !erl.IsAlive(tr.getSelf()) {
		return
	}
	var wg sync.WaitGroup
	wg.Add(1)
	_, err := exitwaiter.New(tr.t, tr.opts.parent, tr.getSelf(), &wg)
	if err != nil {
		tr.t.Errorf("FAILURE starting exitwaiter for test process: %+v\n", err)
		return
	}
	erl.Exit(tr.opts.parent, tr.getSelf(), exitreason.TestExit)
	tr.t.Logf("%s - [%v] waiting for exit\n", time.Now().Format(time.RFC3339Nano), tr)
	wg.Wait()
	tr.t.Logf("test receiver has stopped: %s\n", tr.getSelf())
}

func (tr *TestReceiver) Failures() []*ExpectationFailure {
	tr.failuresMx.RLock()
	defer tr.failuresMx.RUnlock()

	out := make([]*ExpectationFailure, len(tr._failures))

	copy(out, tr._failures)
	return out
}

func (tr *TestReceiver) appendFailure(f *ExpectationFailure) {
	tr.failuresMx.Lock()
	defer tr.failuresMx.Unlock()
	tr._failures = append(tr._failures, f)
}

func (tr *TestReceiver) String() string {
	return fmt.Sprintf("TestReceiver[%v]", tr.getSelf())
}
