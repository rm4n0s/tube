// SPDX-License-Identifier: GPL-2.0-only

package clock

import (
	"sync"
	"time"
)

// Fake is a clock that stands still until a test moves it with Advance. Timers and tickers fire
// inside Advance, in the order of their times, on the goroutine that called it, so that a test
// knows what has happened when Advance returns. It is safe for concurrent use.
type Fake struct {
	mu      sync.Mutex
	now     time.Time
	next    int // the sequence number of the next waiter, to keep the order of equal times
	waiters []*fakeWaiter
}

// fakeWaiter is a pending timer or a ticker of a Fake.
type fakeWaiter struct {
	clock *Fake
	seq   int
	when  time.Time
	// period is the interval of a ticker, and 0 for a timer.
	period time.Duration
	// fire is called, outside the clock's lock, with the time the waiter was due.
	fire func(time.Time)
	// ticks is the channel of a ticker.
	ticks chan time.Time
}

// NewFake returns a fake clock that shows start.
func NewFake(start time.Time) *Fake {
	return &Fake{now: start}
}

// Now returns the time the clock shows.
func (c *Fake) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// add registers a waiter that is due d from now.
func (c *Fake) add(d time.Duration, w *fakeWaiter) {
	c.mu.Lock()
	defer c.mu.Unlock()
	w.clock = c
	w.seq = c.next
	c.next++
	w.when = c.now.Add(d)
	c.waiters = append(c.waiters, w)
}

// remove takes a waiter out and says whether it was pending.
func (c *Fake) remove(w *fakeWaiter) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.removeLocked(w)
}

// removeLocked is remove for a caller that holds the lock.
func (c *Fake) removeLocked(w *fakeWaiter) bool {
	for i, other := range c.waiters {
		if other == w {
			c.waiters = append(c.waiters[:i], c.waiters[i+1:]...)
			return true
		}
	}
	return false
}

// NewTicker returns a ticker that ticks every d of the fake time. It panics when d is not
// positive, as time.NewTicker does.
func (c *Fake) NewTicker(d time.Duration) Ticker {
	if d <= 0 {
		panic("clock: non-positive interval for NewTicker")
	}
	w := &fakeWaiter{period: d, ticks: make(chan time.Time, 1)}
	w.fire = func(now time.Time) {
		select {
		case w.ticks <- now:
		default: // the previous tick was not received: this one is dropped
		}
	}
	c.add(d, w)
	return fakeTicker{w}
}

// AfterFunc calls f during the Advance that reaches d from now, unless the timer is stopped.
func (c *Fake) AfterFunc(d time.Duration, f func()) Timer {
	w := &fakeWaiter{fire: func(time.Time) { f() }}
	c.add(d, w)
	return fakeTimer{w}
}

// Advance moves the clock forward by d and fires every timer and ticker that becomes due, in the
// order of their times (and, for equal times, in the order they were made). While one fires, the
// clock shows the time it was due, and what it does to the clock (a new timer, a stop) counts
// for the rest of the same Advance.
func (c *Fake) Advance(d time.Duration) {
	c.mu.Lock()
	target := c.now.Add(d)
	for {
		var due *fakeWaiter
		for _, w := range c.waiters {
			if w.when.After(target) {
				continue
			}
			if due == nil || w.when.Before(due.when) || (w.when.Equal(due.when) && w.seq < due.seq) {
				due = w
			}
		}
		if due == nil {
			break
		}
		if due.when.After(c.now) {
			c.now = due.when
		}
		if due.period > 0 {
			due.when = due.when.Add(due.period)
		} else {
			c.removeLocked(due)
		}
		c.mu.Unlock()
		due.fire(c.Now())
		c.mu.Lock()
	}
	if target.After(c.now) {
		c.now = target
	}
	c.mu.Unlock()
}

// Pending returns how many timers and tickers are waiting, so that a test can check that
// nothing was left running.
func (c *Fake) Pending() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.waiters)
}

// fakeTicker is a ticker of a Fake.
type fakeTicker struct {
	w *fakeWaiter
}

func (t fakeTicker) C() <-chan time.Time { return t.w.ticks }
func (t fakeTicker) Stop()               { t.w.clock.remove(t.w) }

// fakeTimer is a timer of a Fake.
type fakeTimer struct {
	w *fakeWaiter
}

func (t fakeTimer) Stop() bool { return t.w.clock.remove(t.w) }

func (t fakeTimer) Reset(d time.Duration) bool {
	c := t.w.clock
	c.mu.Lock()
	defer c.mu.Unlock()
	pending := c.removeLocked(t.w)
	t.w.seq = c.next
	c.next++
	t.w.when = c.now.Add(d)
	c.waiters = append(c.waiters, t.w)
	return pending
}
