package asset

import (
	"fmt"
	"time"
)

type Status string

const (
	StatusActive       Status = "active"
	StatusExpiringSoon Status = "expiring"
	StatusExpired      Status = "expired"
	StatusCancelled    Status = "cancelled"
)

var Statuses = []Status{StatusActive, StatusExpiringSoon, StatusExpired, StatusCancelled}

func ParseStatus(s string) (Status, bool) {
	for _, st := range Statuses {
		if string(st) == s {
			return st, true
		}
	}
	return "", false
}

func (s Status) Label() string {
	switch s {
	case StatusActive:
		return "Active"
	case StatusExpiringSoon:
		return "Expiring Soon"
	case StatusExpired:
		return "Expired"
	case StatusCancelled:
		return "Cancelled"
	}
	return string(s)
}

func (s Status) BadgeClass() string {
	switch s {
	case StatusActive:
		return "badge-active"
	case StatusExpiringSoon:
		return "badge-expiring"
	case StatusExpired:
		return "badge-expired"
	}
	return "badge-cancelled"
}

func Date(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func Today(now time.Time, loc *time.Location) time.Time {
	return Date(now.In(loc))
}

func DaysLeft(expiration, today time.Time) int {
	return int(Date(expiration).Sub(Date(today)).Hours() / 24)
}

func MaxOffset(offsets []int32) int {
	m := 0
	for _, o := range offsets {
		if int(o) > m {
			m = int(o)
		}
	}
	return m
}

func ComputeStatus(cancelled bool, expiration, today time.Time, offsets []int32) Status {
	switch {
	case cancelled:
		return StatusCancelled
	case Date(expiration).Before(Date(today)):
		return StatusExpired
	case DaysLeft(expiration, today) <= MaxOffset(offsets):
		return StatusExpiringSoon
	}
	return StatusActive
}

func Countdown(expiration, today time.Time) string {
	d := DaysLeft(expiration, today)
	switch {
	case d == 0:
		return "Expires today"
	case d == 1:
		return "1 day left"
	case d > 1:
		return fmt.Sprintf("%d days left", d)
	case d == -1:
		return "Expired 1 day ago"
	}
	return fmt.Sprintf("Expired %d days ago", -d)
}

func AddMonths(d time.Time, months int) time.Time {
	d = Date(d)
	y, m, day := d.Date()
	target := time.Date(y, m+time.Month(months), 1, 0, 0, 0, 0, time.UTC)
	last := target.AddDate(0, 1, -1).Day()
	if day > last {
		day = last
	}
	return time.Date(target.Year(), target.Month(), day, 0, 0, 0, 0, time.UTC)
}
