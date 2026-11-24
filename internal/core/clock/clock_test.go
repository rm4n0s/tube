// SPDX-License-Identifier: GPL-2.0-only

package clock

import (
	"reflect"
	"sync"
	"testing"
	"time"
)

var start = time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)

func TestFakeNowAndAdvance(t *testing.T) {
	c := NewFake(start)
	if !c.Now().Equal(start) {
		t.Errorf("Now() = %v, want %v", c.Now(), start)
	}
	c.Advance(90 * time.Second)
	if want := start.Add(90 * time.Second); !c.Now().Equal(want) {
		t.Errorf("Now() = %v, want %v", c.Now(), want)
	}
	c.Advance(0)
	if want := start.Add(90 * time.Second); !c.Now().Equal(want) {
		t.Errorf("after Advance(0): Now() = %v, want %v", c.Now(), want)
	}
}

func TestFakeTimersFireInOrder(t *testing.T) {
	tests := []struct {
		name string
		// delays are the timers, in the order they are made.
		delays  []time.Duration
		advance time.Duration
		// want are the indexes of the timers that fire, in order.
		want        []int
		wantPending int
	}{
		{"by time, not by creation", []time.Duration{3 * time.Second, time.Second, 2 * time.Second}, 5 * time.Second, []int{1, 2, 0}, 0},
		{"equal times by creation", []time.Duration{time.Second, time.Second, time.Second}, time.Second, []int{0, 1, 2}, 0},
		{"only those that are due", []time.Duration{time.Second, 10 * time.Second}, 5 * time.Second, []int{0}, 1},
		{"exactly at the time", []time.Duration{5 * time.Second}, 5 * time.Second, []int{0}, 0},
		{"just before the time", []time.Duration{5 * time.Second}, 5*time.Second - 1, nil, 1},
		{"zero delay fires at the next advance", []time.Duration{0}, 0, []int{0}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewFake(start)
			var fired []int
			var at []time.Time
			for i, d := range tt.delays {
				c.AfterFunc(d, func() {
					fired = append(fired, i)
					at = append(at, c.Now())
				})
			}
			c.Advance(tt.advance)
			if !reflect.DeepEqual(fired, tt.want) {
				t.Errorf("fired %v, want %v", fired, tt.want)
			}
			for n, i := range fired {
				if want := start.Add(tt.delays[i]); !at[n].Equal(want) {
					t.Errorf("timer %d saw the time %v, want %v", i, at[n], want)
				}
			}
			if c.Pending() != tt.wantPending {
				t.Errorf("Pending() = %d, want %d", c.Pending(), tt.wantPending)
			}
			if want := start.Add(tt.advance); !c.Now().Equal(want) {
				t.Errorf("Now() = %v after the advance, want %v", c.Now(), want)
			}
		})
	}
}

func TestFakeTimerStopAndReset(t *testing.T) {
	c := NewFake(start)
	fired := 0
	timer := c.AfterFunc(time.Minute, func() { fired++ })
	if !timer.Stop() {
		t.Error("Stop() of a pending timer = false")
	}
	if timer.Stop() {
		t.Error("a second Stop() = true")
	}
	c.Advance(time.Hour)
	if fired != 0 {
		t.Errorf("a stopped timer fired %d times", fired)
	}

	if timer.Reset(time.Minute) {
		t.Error("Reset() of a stopped timer = true")
	}
	c.Advance(30 * time.Second)
	if !timer.Reset(time.Minute) {
		t.Error("Reset() of a pending timer = false")
	}
	c.Advance(45 * time.Second) // 75 s after the first reset, 45 s after the second
	if fired != 0 {
		t.Error("the timer fired at its old time")
	}
	c.Advance(15 * time.Second)
	if fired != 1 {
		t.Errorf("fired %d times, want 1", fired)
	}
	if timer.Stop() {
		t.Error("Stop() of a timer that fired = true")
	}
	if c.Pending() != 0 {
		t.Errorf("Pending() = %d", c.Pending())
	}
}

func TestFakeTimerMadeWhileFiring(t *testing.T) {
	c := NewFake(start)
	var order []string
	c.AfterFunc(time.Second, func() {
		order = append(order, "first")
		// Due inside the same advance: it fires in it, after its turn comes.
		c.AfterFunc(time.Second, func() { order = append(order, "chained at "+c.Now().Sub(start).String()) })
	})
	c.AfterFunc(1500*time.Millisecond, func() { order = append(order, "second") })
	c.Advance(10 * time.Second)
	want := []string{"first", "second", "chained at 2s"}
	if !reflect.DeepEqual(order, want) {
		t.Errorf("order = %v, want %v", order, want)
	}
}

func TestFakeTicker(t *testing.T) {
	c := NewFake(start)
	ticker := c.NewTicker(time.Minute)

	c.Advance(59 * time.Second)
	select {
	case tick := <-ticker.C():
		t.Fatalf("a tick at %v before the interval", tick)
	default:
	}
	c.Advance(time.Second)
	select {
	case tick := <-ticker.C():
		if want := start.Add(time.Minute); !tick.Equal(want) {
			t.Errorf("tick = %v, want %v", tick, want)
		}
	default:
		t.Fatal("no tick after the interval")
	}

	// Three intervals without a receive: one tick is kept, the others are dropped.
	c.Advance(3 * time.Minute)
	select {
	case tick := <-ticker.C():
		if want := start.Add(2 * time.Minute); !tick.Equal(want) {
			t.Errorf("the kept tick = %v, want %v", tick, want)
		}
	default:
		t.Fatal("no tick was kept")
	}
	select {
	case tick := <-ticker.C():
		t.Fatalf("a second tick %v was kept", tick)
	default:
	}

	ticker.Stop()
	ticker.Stop() // stopping twice is fine
	c.Advance(time.Hour)
	select {
	case tick := <-ticker.C():
		t.Fatalf("a tick %v after Stop", tick)
	default:
	}
	if c.Pending() != 0 {
		t.Errorf("Pending() = %d", c.Pending())
	}
}

func TestFakeTickerNeedsAPositiveInterval(t *testing.T) {
	for _, d := range []time.Duration{0, -time.Second} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("NewTicker(%v) did not panic", d)
				}
			}()
			NewFake(start).NewTicker(d)
		}()
	}
}

func TestFakeIsSafeForConcurrentUse(t *testing.T) {
	c := NewFake(start)
	var mu sync.Mutex
	fired := 0
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				timer := c.AfterFunc(time.Millisecond, func() {
					mu.Lock()
					fired++
					mu.Unlock()
				})
				c.Now()
				c.Advance(time.Millisecond)
				timer.Stop()
			}
		}()
	}
	wg.Wait()
	c.Advance(time.Second)
	mu.Lock()
	defer mu.Unlock()
	if fired == 0 || fired > 800 {
		t.Errorf("fired = %d", fired)
	}
	if c.Pending() != 0 {
		t.Errorf("Pending() = %d", c.Pending())
	}
}

func TestSystemClock(t *testing.T) {
	c := System()
	before := time.Now()
	if now := c.Now(); now.Before(before) || now.After(before.Add(time.Minute)) {
		t.Errorf("Now() = %v is not the system's time %v", now, before)
	}

	done := make(chan struct{})
	timer := c.AfterFunc(time.Millisecond, func() { close(done) })
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("AfterFunc did not fire")
	}
	if timer.Stop() {
		t.Error("Stop() of a timer that fired = true")
	}
	stopped := c.AfterFunc(time.Hour, func() { t.Error("a stopped timer fired") })
	if !stopped.Stop() {
		t.Error("Stop() of a pending timer = false")
	}

	ticker := c.NewTicker(time.Millisecond)
	select {
	case <-ticker.C():
	case <-time.After(10 * time.Second):
		t.Fatal("the ticker did not tick")
	}
	ticker.Stop()
}
