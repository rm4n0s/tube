// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"strconv"
	"strings"
	"time"
)

// byteUnits are the decimal units of bytesText, from kB on.
var byteUnits = []string{"kB", "MB", "GB", "TB", "PB", "EB"}

// bytesText is a size in decimal units: "0 B", "999 B", "1.0 kB", "1.5 MB", "2.3 GB".
func bytesText(n uint64) string {
	if n < 1000 {
		return strconv.FormatUint(n, 10) + " B"
	}
	value := float64(n) / 1000
	unit := 0
	// The value is rounded to one digit before it is compared, so that 999,950 bytes is
	// "1.0 MB", not "1000.0 kB".
	for unit < len(byteUnits)-1 && value*10+0.5 >= 10000 {
		value /= 1000
		unit++
	}
	return strconv.FormatFloat(value, 'f', 1, 64) + " " + byteUnits[unit]
}

// rateText is a rate in bytes a second: bytesText and "/s".
func rateText(bytesPerSecond float64) string {
	if bytesPerSecond < 0 {
		bytesPerSecond = 0
	}
	return bytesText(uint64(bytesPerSecond+0.5)) + "/s"
}

// durationText is a time as "00:00:05", "01:02:03", or "2 days 03:04:05".
func durationText(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	s := int64(d / time.Second)
	days, s := s/86400, s%86400
	clock := pad2(s/3600) + ":" + pad2(s/60%60) + ":" + pad2(s%60)
	switch days {
	case 0:
		return clock
	case 1:
		return "1 day " + clock
	}
	return strconv.FormatInt(days, 10) + " days " + clock
}

// pad2 is a number of at least two digits.
func pad2(n int64) string {
	if n < 10 {
		return "0" + strconv.FormatInt(n, 10)
	}
	return strconv.FormatInt(n, 10)
}

// countText is a number with commas between the thousands: "1,234,567".
func countText(n uint64) string {
	digits := strconv.FormatUint(n, 10)
	var b strings.Builder
	for i, d := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(d)
	}
	return b.String()
}
