package bizday

import "time"

// FirstOfMonth returns the first day of the month n months after d's month.
func FirstOfMonth(d Date, n int) Date {
	t := time.Date(d.Year, d.Month, 1, 0, 0, 0, 0, time.UTC).AddDate(0, n, 0)
	return FromTime(t)
}

// LastOfMonth returns the last day of d's month.
func LastOfMonth(d Date) Date { return FirstOfMonth(d, 1).AddDays(-1) }
