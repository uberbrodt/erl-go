package timeout

import (
	"time"

	"github.com/uberbrodt/erl-go/chronos"
)

const (
	InfinityInt int           = -37
	Infinity    time.Duration = 1<<63 - 1
)

var Default time.Duration = chronos.Dur("5s")
