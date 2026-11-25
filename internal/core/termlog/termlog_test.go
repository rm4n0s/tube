// SPDX-License-Identifier: GPL-2.0-only

package termlog

import (
	"math/rand/v2"
	"slices"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

// all returns every line that the log keeps.
func all(l *Log) []string {
	first, end := l.Len()
	return l.Range(first, end)
}

func TestLines(t *testing.T) {
	// The first 256 bytes: ESC, "]" and 254 bytes "A".
	never := "\x1b]" + strings.Repeat("A", 254) + strings.Repeat("b", 744)
	tests := []struct {
		name   string
		writes []string
		want   []string
	}{
		{"plain lines", []string{"one\ntwo\n"}, []string{"one", "two"}},
		{"a line without an ending", []string{"one\ntwo"}, []string{"one", "two"}},
		{"CR LF", []string{"one\r\ntwo\r\n"}, []string{"one", "two"}},
		{"an empty line", []string{"one\n\ntwo\n"}, []string{"one", "", "two"}},
		{"a progress bar", []string{"10%\r20%\r30%\n"}, []string{"30%"}},
		{"a carriage return empties the line", []string{"abc\rx\n"}, []string{"x"}},
		{"backspace", []string{"abd\bc\n"}, []string{"abc"}},
		{"a tab after 3 characters", []string{"abc\tx\n"}, []string{"abc     x"}},
		{"colors", []string{"\x1b[31mred\x1b[0m\n"}, []string{"red"}},
		{"a title", []string{"\x1b]0;title\x07text\n"}, []string{"text"}},
		{"a title that ends with ESC backslash", []string{"\x1b]0;title\x1b\\text\n"}, []string{"text"}},
		{"cursor moves", []string{"\x1b[2J\x1b[H"}, nil},
		{"a DCS string", []string{"a\x1bPq#0;2;0;0;0\x1b\\b\n"}, []string{"ab"}},
		{"ESC and one byte", []string{"a\x1b7b\x1b=c\n"}, []string{"abc"}},
		{"a sequence cut in two writes", []string{"\x1b[", "31mred\n"}, []string{"red"}},
		{"a character cut in two writes", []string{"caf\xc3", "\xa9\n"}, []string{"café"}},
		{"a sequence that never ends", []string{never, "\nok\n"}, []string{strings.Repeat("b", 744), "ok"}},
		{"invalid UTF-8", []string{"a\xffb\xc3(\n"}, []string{"a\ufffdb\ufffd("}},
		{"a bidirectional override", []string{"a\u202eb\u2066c\n"}, []string{"abc"}},
		{"a C1 control", []string{"a\u0085b\u009bc\n"}, []string{"abc"}},
		{"line and paragraph separators", []string{"a\u2028b\u2029c\n"}, []string{"abc"}},
		{"other control characters", []string{"a\x00\x01\x7fb\x07c\n"}, []string{"abc"}},
		{"a CSI with a new line inside", []string{"a\x1b[3\nb\n"}, []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := New(100, 1000)
			for _, w := range tt.writes {
				if n, err := l.Write([]byte(w)); n != len(w) || err != nil {
					t.Fatalf("Write = %d, %v", n, err)
				}
			}
			if got := all(l); !slices.Equal(got, tt.want) {
				t.Errorf("the lines are %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTheWidth(t *testing.T) {
	l := New(10, 20)
	if _, err := l.Write([]byte(strings.Repeat("x", 30) + "\nwhole\n")); err != nil {
		t.Fatal(err)
	}
	if got := all(l); !slices.Equal(got, []string{strings.Repeat("x", 20), "whole"}) {
		t.Errorf("the lines are %q", got)
	}
}

func TestTheLimit(t *testing.T) {
	l := New(3, 80)
	if _, err := l.Write([]byte("1\n2\n3\n4\n5\n")); err != nil {
		t.Fatal(err)
	}
	if first, end := l.Len(); first != 2 || end != 5 {
		t.Errorf("Len = %d, %d", first, end)
	}
	if got := l.Range(0, 5); !slices.Equal(got, []string{"3", "4", "5"}) {
		t.Errorf("Range(0, 5) = %q", got)
	}
}

func TestRange(t *testing.T) {
	l := New(3, 80)
	if _, err := l.Write([]byte("1\n2\n3\n4\n5")); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		from, to int
		want     []string
	}{
		{-10, 100, []string{"2", "3", "4", "5"}}, // three finished lines, and the unfinished one
		{3, 4, []string{"4"}},
		{4, 5, []string{"5"}},
		{4, 4, nil},
		{5, 3, nil},
		{100, 200, nil},
	}
	for _, tt := range tests {
		if got := l.Range(tt.from, tt.to); !slices.Equal(got, tt.want) {
			t.Errorf("Range(%d, %d) = %q, want %q", tt.from, tt.to, got, tt.want)
		}
	}
	if got := New(3, 80).Range(0, 10); got != nil {
		t.Errorf("Range of an empty log = %q", got)
	}
}

func TestOnWrite(t *testing.T) {
	l := New(10, 80)
	calls := 0
	l.OnWrite(func() { calls++ })
	for _, w := range []string{"text", "", "\x1b[31m"} {
		if _, err := l.Write([]byte(w)); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Errorf("OnWrite was called %d times, want once", calls)
	}
}

// safe says whether a line has nothing that a terminal would take for a command.
func safe(line string) bool {
	if !utf8.ValidString(line) {
		return false
	}
	for i := 0; i < len(line); i++ {
		if line[i] < 0x20 || line[i] == 0x7f {
			return false
		}
	}
	for _, r := range line {
		if dropped(r) {
			return false
		}
	}
	return true
}

func TestNothingUnsafeComesOut(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	l := New(5000, 200)
	data := make([]byte, 64<<10)
	for i := range data {
		data[i] = byte(rng.Uint32())
	}
	for len(data) > 0 {
		n := min(len(data), 1+rng.IntN(300))
		if _, err := l.Write(data[:n]); err != nil {
			t.Fatal(err)
		}
		data = data[n:]
	}
	lines := all(l)
	if len(lines) == 0 {
		t.Fatal("no line came out")
	}
	for _, line := range lines {
		if !safe(line) {
			t.Fatalf("an unsafe line: %q", line)
		}
	}
	for _, line := range strings.Split(l.String(), "\n") {
		if !safe(line) {
			t.Fatalf("String has an unsafe line: %q", line)
		}
	}
}

func TestConcurrentUse(t *testing.T) {
	l := New(100, 80)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range 1000 {
			if _, err := l.Write([]byte("line " + strings.Repeat("x", i%50) + "\n")); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for range 1000 {
			first, end := l.Len()
			for _, line := range l.Range(first, end) {
				if !strings.HasPrefix(line, "line ") {
					t.Errorf("a line that was not written: %q", line)
					return
				}
			}
		}
	}()
	wg.Wait()
}
