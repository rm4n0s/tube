// SPDX-License-Identifier: GPL-2.0-only

// Package clock is the time Tube's core runs on: the system's in the programs, and a fake that
// tests move by hand (docs/SPEC.md §14.3).
package clock

import "time"

// Clock tells the time and makes timers.
type Clock interface {
	// Now returns the current time.
	Now() time.Time
	// NewTicker returns a ticker that sends the time every d, which must be positive. As with
	// time.Ticker, a tick is dropped when the previous one was not received.
	NewTicker(d time.Duration) Ticker
	// AfterFunc calls f once, d from now, unless the timer is stopped first.
	AfterFunc(d time.Duration, f func()) Timer
}

// Ticker sends the time at intervals.
type Ticker interface {
	// C returns the channel the ticks arrive on.
	C() <-chan time.Time
	// Stop ends the ticks. It does not close the channel.
	Stop()
}

// Timer is a pending call made by AfterFunc.
type Timer interface {
	// Stop cancels the call and says whether it was still pending.
	Stop() bool
	// Reset makes the call happen d from now, and says whether it was still pending.
	Reset(d time.Duration) bool
}

// System returns the clock of the operating system.
func System() Clock {
	return systemClock{}
}

// systemClock is the real clock.
type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

func (systemClock) NewTicker(d time.Duration) Ticker {
	return systemTicker{time.NewTicker(d)}
}

func (systemClock) AfterFunc(d time.Duration, f func()) Timer {
	return time.AfterFunc(d, f)
}

// systemTicker is a time.Ticker as a Ticker.
type systemTicker struct {
	ticker *time.Ticker
}

func (t systemTicker) C() <-chan time.Time { return t.ticker.C }
func (t systemTicker) Stop()               { t.ticker.Stop() }
