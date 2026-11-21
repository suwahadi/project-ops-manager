package auth

import "time"

const (
	MaxFailedLogins = 5
	LockoutWindow   = 15 * time.Minute
	LockoutDuration = 15 * time.Minute
)

type Attempt struct {
	Success bool
	At      time.Time
}

func LockedUntil(attempts []Attempt, now time.Time) (time.Time, bool) {
	var failures []time.Time
	for _, a := range attempts {
		if a.Success {
			failures = failures[:0]
			continue
		}
		failures = append(failures, a.At)
	}
	var until time.Time
	for i := 0; i+MaxFailedLogins-1 < len(failures); i++ {
		last := failures[i+MaxFailedLogins-1]
		if last.Sub(failures[i]) <= LockoutWindow {
			if end := last.Add(LockoutDuration); end.After(until) {
				until = end
			}
		}
	}
	if now.Before(until) {
		return until, true
	}
	return time.Time{}, false
}
