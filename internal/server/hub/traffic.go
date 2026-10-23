package hub

import (
	"time"

	"github.com/mby/fenghuo/internal/server/store"
)

// bootJitter tolerates small drifts of the kernel's computed boot time.
const bootJitter = 60

// applyCounters adds the traffic since the previous report to t, using the
// agent's cumulative since-boot counters.
func applyCounters(t *store.Traffic, boot int64, rx, tx uint64) {
	sameBoot := t.BootTime != 0 && abs(boot-t.BootTime) <= bootJitter
	switch {
	case t.BootTime == 0 && t.LastRx == 0 && t.LastTx == 0:
		// First report ever: start counting from here.
	case sameBoot && rx >= t.LastRx && tx >= t.LastTx:
		t.Down += rx - t.LastRx
		t.Up += tx - t.LastTx
	case sameBoot:
		// Counters went backwards without a reboot (interface removed or the
		// agent's interface list changed): rebase without adding anything.
	default:
		// The host rebooted; everything counted since boot is new traffic.
		t.Down += rx
		t.Up += tx
	}
	t.BootTime, t.LastRx, t.LastTx = boot, rx, tx
}

// PeriodStart returns the start of the billing period containing now, for a
// period that resets on the given day of month (1-31) at midnight in now's
// location. Short months use their last day without changing the configured day.
func PeriodStart(now time.Time, resetDay int) int64 {
	y, m, _ := now.Date()
	start := resetInMonth(y, m, resetDay, now.Location())
	if now.Before(start) {
		start = resetInMonth(y, m-1, resetDay, now.Location())
	}
	return start.Unix()
}

// NextPeriodStart returns the first reset strictly after now, retaining its
// location so callers can use the UTC offset at that future instant (including DST).
func NextPeriodStart(now time.Time, resetDay int) time.Time {
	y, m, _ := now.Date()
	next := resetInMonth(y, m, resetDay, now.Location())
	if !now.Before(next) {
		next = resetInMonth(y, m+1, resetDay, now.Location())
	}
	return next
}

func resetInMonth(year int, month time.Month, resetDay int, loc *time.Location) time.Time {
	if resetDay < 1 || resetDay > 31 {
		resetDay = 1
	}
	// Rebuild each month from the original day; adding months to Jan 31 or a
	// clamped February date either overflows or loses the original reset day.
	days := time.Date(year, month+1, 0, 12, 0, 0, 0, time.UTC).Day()
	return time.Date(year, month, min(resetDay, days), 0, 0, 0, 0, loc)
}

// TrafficUsed returns the billable traffic for a mode: sum, up, down or max.
func TrafficUsed(t store.Traffic, mode string) uint64 {
	switch mode {
	case "up":
		return t.Up
	case "down":
		return t.Down
	case "max":
		return max(t.Up, t.Down)
	default:
		return t.Up + t.Down
	}
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
