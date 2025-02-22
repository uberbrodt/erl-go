package erltest_test

import (
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/uberbrodt/erl-go/erl/x/erltest/internal/mock"
)

// Use when testing the [erltest.TestReceiver] and assure that it fails a test when expected.
//
// It sets up [erltest.AnyTimes] expectations on all the methods used. If you want to assert
// that it calls the [*testing.T] in a certain way, pass it an [expects] function reference
// that will set expectations ahead of the default expectations.
//
// # Example
//
//	exs := func(m *mock.MockTLike) {
//		m.EXPECT().Error("DoFailIt").Times(1)
//	}
//	fakeT := standardFakeT(t, &exs)
func standardFakeT(t *testing.T, expects *func(ft *mock.MockTLike)) *mock.MockTLike {
	ctrl := gomock.NewController(t)
	fakeT := mock.NewMockTLike(ctrl)

	if expects != nil {
		e := *expects
		e(fakeT)
	}

	fakeT.EXPECT().Cleanup(gomock.Any()).AnyTimes()
	fakeT.EXPECT().Deadline().Return(time.Now().Add(3*time.Second), true).Times(1)
	fakeT.EXPECT().Log(gomock.Any()).AnyTimes()
	fakeT.EXPECT().Logf(gomock.Any(), gomock.Any()).AnyTimes()
	fakeT.EXPECT().Helper().AnyTimes()
	fakeT.EXPECT().Failed().AnyTimes()
	fakeT.EXPECT().Errorf(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	fakeT.EXPECT().Errorf(gomock.Any(), gomock.Any()).AnyTimes()
	fakeT.EXPECT().Error(gomock.Any()).AnyTimes()

	return fakeT
}
