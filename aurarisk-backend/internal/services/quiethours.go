package services

import (
	"fmt"
	"time"
)

type QuietHours struct {
	Start    string `json:"start"`
	End      string `json:"end"`
	Timezone string `json:"timezone"`
}

func parseClock(value string) (int, error) {
	t, err := time.Parse("15:04", value)
	if err != nil {
		return 0, fmt.Errorf("invalid time %q, expected HH:MM", value)
	}
	return t.Hour()*60 + t.Minute(), nil
}

func (q QuietHours) Validate() error {
	start, err := parseClock(q.Start)
	if err != nil {
		return err
	}
	end, err := parseClock(q.End)
	if err != nil {
		return err
	}
	if start == end {
		return fmt.Errorf("quiet hours start and end must differ")
	}
	if q.Timezone == "" {
		return fmt.Errorf("quiet hours timezone is required")
	}
	if _, err := time.LoadLocation(q.Timezone); err != nil {
		return fmt.Errorf("invalid timezone %q", q.Timezone)
	}
	return nil
}

// Contains reports whether t falls inside the quiet window in the configured
// timezone. Windows where end is before start wrap past midnight.
func (q QuietHours) Contains(t time.Time) bool {
	start, err := parseClock(q.Start)
	if err != nil {
		return false
	}
	end, err := parseClock(q.End)
	if err != nil {
		return false
	}
	loc, err := time.LoadLocation(q.Timezone)
	if err != nil {
		return false
	}

	local := t.In(loc)
	minute := local.Hour()*60 + local.Minute()

	if start < end {
		return minute >= start && minute < end
	}
	return minute >= start || minute < end
}

var levelRank = map[string]int{
	"Normal":    0,
	"Advisory":  1,
	"Alert":     2,
	"Emergency": 3,
}

// LevelRank orders risk levels; unknown levels rank below Normal.
func LevelRank(level string) int {
	if rank, ok := levelRank[level]; ok {
		return rank
	}
	return -1
}
