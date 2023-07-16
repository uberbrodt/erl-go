package chronos

import (
	"time"
	_ "time/tzdata"
)

// Returns current time, if [tz] is "" or "UTC", then returns as UTC
// If [tz] is "LOCAL", returns [time.Time] in current local time
//
// Othwerwise, [tz] can be any valid IANA timezone db file name.
// eg: "America/Chicago"
func Now(tz string) time.Time {
	loc, _ := time.LoadLocation(tz)
	return time.Now().In(loc)
}

func Dur(s string) time.Duration {
	t, err := time.ParseDuration(s)
	if err != nil {
		panic(err)
	}
	return t
}

// Converts [tz] to a timezone
func In(t time.Time, tz string) time.Time {
	loc, _ := time.LoadLocation(tz)
	return t.In(loc)
}
