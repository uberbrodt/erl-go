package erltest

import (
	"fmt"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/genserver"
)

type (
	BoolTestExpectation     func(self erl.PID, msg any) bool
	BoolTestCallExpectation func(self erl.PID, from genserver.From, msg any) bool
)

type ExpectArg struct {
	// The term that was matched against. A simple type in the case of [reflect.Type], or
	// possibly a string, number, etc.
	Match any
	Msg   any
	// This is only populated for Calls
	From     *genserver.From
	Self     erl.PID
	MsgCount int
	Exp      Expectation
}

func Fail(ea ExpectArg, reason string) *ExpectationFailure {
	return &ExpectationFailure{
		Exp:    ea.Exp,
		Msg:    ea.Msg,
		Reason: reason,
	}
}

type Expectation interface {
	Check(arg ExpectArg) (Expectation, *ExpectationFailure)
	Satisfied(testDone bool) bool
	// unique identifier, to simplify nested Expectation registration with the [TestReceiver]
	ID() string
}

type ExpectationFailure struct {
	// The matching term that led to the ExpectationFailure
	Match any
	// The failed expectation
	Exp Expectation
	// The actual message that matched [Match] and was evaluated by the Expectation
	Msg any
	// Failure message
	Reason string
}

func (ef *ExpectationFailure) String() string {
	return fmt.Sprintf("Expectation Failed: %s, [%v]", ef.Reason, ef.Exp)
}
