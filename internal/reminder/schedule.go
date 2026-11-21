package reminder

import (
	"sort"
	"time"

	"github.com/suwahadi/project-ops-manager/internal/asset"
)

const (
	ChannelTelegram = "telegram"
	ChannelEmail    = "email"
)

var Channels = []string{ChannelTelegram, ChannelEmail}

type Clock struct {
	Now      time.Time
	LastRun  *time.Time
	Location *time.Location
	Hour     int
	Minute   int
}

func (c Clock) Today() time.Time { return asset.Today(c.Now, c.Location) }

func DueDate(expiration time.Time, offset int32) time.Time {
	return asset.Date(expiration).AddDate(0, 0, -int(offset))
}

func (c Clock) Moment(day time.Time) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), c.Hour, c.Minute, 0, 0, c.Location)
}

type Key struct {
	AssetID    string
	Expiration time.Time
	Offset     int32
	Channel    string
}

func SelectOffset(expiration time.Time, offsets []int32, c Clock) (int32, bool) {
	today := c.Today()
	if !asset.Date(expiration).After(today) {
		return 0, false
	}
	best := int32(-1)
	for _, o := range offsets {
		if o < 1 {
			continue
		}
		due := DueDate(expiration, o)
		moment := c.Moment(due)
		if moment.After(c.Now) {
			continue
		}
		dueToday := due.Equal(today)
		missedSinceLastRun := c.LastRun != nil && moment.After(*c.LastRun)
		if !dueToday && !missedSinceLastRun {
			continue
		}
		if best == -1 || o < best {
			best = o
		}
	}
	if best == -1 {
		return 0, false
	}
	return best, true
}

type Planned struct {
	Offset int32
	Due    time.Time
}

func Plan(expiration time.Time, offsets []int32) []Planned {
	sorted := append([]int32(nil), offsets...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] > sorted[j] })
	out := make([]Planned, 0, len(sorted))
	for _, o := range sorted {
		out = append(out, Planned{Offset: o, Due: DueDate(expiration, o)})
	}
	return out
}

var RetryBackoff = []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute}

func NextAttempt(attemptsMade int, now time.Time) (time.Time, bool) {
	if attemptsMade < 1 || attemptsMade > len(RetryBackoff) {
		return time.Time{}, false
	}
	return now.Add(RetryBackoff[attemptsMade-1]), true
}
