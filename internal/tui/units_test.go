// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"testing"
	"time"
)

func TestBytesText(t *testing.T) {
	tests := []struct {
		n    uint64
		want string
	}{
		{0, "0 B"}, {999, "999 B"}, {1000, "1.0 kB"}, {1049, "1.0 kB"}, {1050, "1.1 kB"},
		{999_949, "999.9 kB"}, {999_950, "1.0 MB"}, {1_500_000, "1.5 MB"}, {80_000_000, "80.0 MB"},
		{2_300_000_000, "2.3 GB"}, {1_000_000_000_000, "1.0 TB"}, {^uint64(0), "18.4 EB"},
	}
	for _, tt := range tests {
		if got := bytesText(tt.n); got != tt.want {
			t.Errorf("bytesText(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestRateText(t *testing.T) {
	tests := []struct {
		rate float64
		want string
	}{
		{0, "0 B/s"}, {-5, "0 B/s"}, {999.4, "999 B/s"}, {999.6, "1.0 kB/s"}, {80_000, "80.0 kB/s"}, {1_200_000, "1.2 MB/s"},
	}
	for _, tt := range tests {
		if got := rateText(tt.rate); got != tt.want {
			t.Errorf("rateText(%v) = %q, want %q", tt.rate, got, tt.want)
		}
	}
}

func TestDurationText(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "00:00:00"}, {-time.Second, "00:00:00"}, {5 * time.Second, "00:00:05"}, {59 * time.Second, "00:00:59"},
		{60 * time.Second, "00:01:00"}, {time.Hour + 2*time.Minute + 3*time.Second, "01:02:03"},
		{24*time.Hour - time.Second, "23:59:59"}, {24 * time.Hour, "1 day 00:00:00"},
		{2*24*time.Hour + 3*time.Hour + 4*time.Minute + 5*time.Second, "2 days 03:04:05"},
		{1500 * time.Millisecond, "00:00:01"},
	}
	for _, tt := range tests {
		if got := durationText(tt.d); got != tt.want {
			t.Errorf("durationText(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestCountText(t *testing.T) {
	tests := []struct {
		n    uint64
		want string
	}{
		{0, "0"}, {999, "999"}, {1000, "1,000"}, {1_234_567, "1,234,567"}, {100_000, "100,000"},
	}
	for _, tt := range tests {
		if got := countText(tt.n); got != tt.want {
			t.Errorf("countText(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}
