package erl_test

import (
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/uberbrodt/erl-go/erl"
	"github.com/uberbrodt/erl-go/erl/x/erltest"
	"github.com/uberbrodt/erl-go/erl/x/erltest/testcase"
)

type TestMsg struct {
	Name string
}

func TestExtMessageOrdering(t *testing.T) {
	tc := testcase.New(t, erltest.WaitTimeout(0))

	tc.Arrange(func(self erl.PID) {
		// tc.Receiver().Expect(TestMsg{}, expectMsgs(expected))
		blueEx := tc.Receiver().Expect(TestMsg{}, gomock.Eq(TestMsg{Name: "Blue"}))
		greenEx := tc.Receiver().Expect(TestMsg{}, gomock.Eq(TestMsg{Name: "Green"}))
		redEx := tc.Receiver().Expect(TestMsg{}, gomock.Eq(TestMsg{Name: "Red"}))
		orangeEx := tc.Receiver().Expect(TestMsg{}, gomock.Eq(TestMsg{Name: "Orange"}))
		blackEx := tc.Receiver().Expect(TestMsg{}, gomock.Eq(TestMsg{Name: "Black"}))
		purpleEx := tc.Receiver().Expect(TestMsg{}, gomock.Eq(TestMsg{Name: "Purple"}))
		yellowEx := tc.Receiver().Expect(TestMsg{}, gomock.Eq(TestMsg{Name: "Yellow"}))
		chartruseEx := tc.Receiver().Expect(TestMsg{}, gomock.Eq(TestMsg{Name: "Chartruse"}))
		chartruseEx.After(
			yellowEx.After(
				purpleEx.After(
					blackEx.After(
						orangeEx.After(
							redEx.After(
								greenEx.After(
									blueEx)))))))
	})

	tc.Act(func() {
		erl.Send(tc.TestPID(), TestMsg{Name: "Blue"})
		erl.Send(tc.TestPID(), TestMsg{Name: "Green"})
		erl.Send(tc.TestPID(), TestMsg{Name: "Red"})
		erl.Send(tc.TestPID(), TestMsg{Name: "Orange"})
		erl.Send(tc.TestPID(), TestMsg{Name: "Black"})
		erl.Send(tc.TestPID(), TestMsg{Name: "Purple"})
		erl.Send(tc.TestPID(), TestMsg{Name: "Yellow"})
		erl.Send(tc.TestPID(), TestMsg{Name: "Chartruse"})
	})

	tc.Assert(func() {
	})
}
