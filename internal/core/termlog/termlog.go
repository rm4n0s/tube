// SPDX-License-Identifier: GPL-2.0-only

// Package termlog keeps what a program writes to a terminal as plain lines that are safe to draw
// (docs/SPEC.md §13.9): escape sequences and control characters are dropped, a carriage return
// lets a line be written again (a progress bar), and the lines are bounded in number and width.
// No string that it returns has a byte below 0x20, a 0x7F, or an escape character in it.
package termlog

import (
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// The bytes that the log reads.
const (
	esc       = 0x1b
	bel       = 0x07
	backspace = 0x08
	del       = 0x7f
	tabWidth  = 8
	// maxSequence is the length after which an escape sequence that has not ended is given up.
	maxSequence = 256
)

// state is where the reading of an escape sequence is.
type state int

// The states of the reading.
const (
	stateText   state = iota // not in a sequence
	stateEsc                 // after ESC
	stateCSI                 // after ESC [
	stateOSC                 // after ESC ], until BEL or ESC \
	stateOSCEsc              // an ESC inside an OSC
	stateStr                 // after ESC P, X, ^ or _, until ESC \
	stateStrEsc              // an ESC inside such a string
)

// Log keeps the last lines that a program wrote to a terminal, as plain text.
type Log struct {
	mu       sync.Mutex
	maxLines int
	maxWidth int
	// lines are the finished lines; first is the number of lines[0].
	lines []string
	first int
	// cur is the unfinished line; overwrite says that a carriage return came, and that the next
	// printable character empties the line first.
	cur       []rune
	overwrite bool
	// utf8 holds the bytes of a character that is not complete yet.
	utf8 []byte
	// state is the reading of an escape sequence, and seq its length so far.
	state state
	seq   int
	// changed says whether the Write in progress changed the log.
	changed bool
	onWrite func()
}

// New makes a log that keeps at most maxLines lines of at most maxWidth characters.
func New(maxLines, maxWidth int) *Log {
	return &Log{maxLines: max(maxLines, 1), maxWidth: max(maxWidth, 1)}
}

// OnWrite sets a function that is called after each Write that changed the log. It is called
// without the log's lock. Set it before the log is used.
func (l *Log) OnWrite(f func()) { l.onWrite = f }

// Write takes what the program wrote. It never fails.
func (l *Log) Write(p []byte) (int, error) {
	l.mu.Lock()
	l.changed = false
	for _, b := range p {
		l.byte(b)
	}
	changed := l.changed
	l.mu.Unlock()
	if changed && l.onWrite != nil {
		l.onWrite()
	}
	return len(p), nil
}

// Len tells which lines the log has: first is the number of the oldest line that is kept, end
// is the number after the last. Lines are numbered from 0 since the log was made; the unfinished
// line counts when it is not empty.
func (l *Log) Len() (first, end int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.first, l.end()
}

// end is the number after the last line; the caller holds the lock.
func (l *Log) end() int {
	end := l.first + len(l.lines)
	if len(l.cur) > 0 {
		end++
	}
	return end
}

// Range returns the lines from number from up to, not including, number to, as far as they are
// kept.
func (l *Log) Range(from, to int) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	from, to = max(from, l.first), min(to, l.end())
	if from >= to {
		return nil
	}
	out := make([]string, 0, to-from)
	for n := from; n < to; n++ {
		if i := n - l.first; i < len(l.lines) {
			out = append(out, l.lines[i])
		} else {
			out = append(out, string(l.cur))
		}
	}
	return out
}

// byte reads one byte.
func (l *Log) byte(b byte) {
	if l.state != stateText {
		l.sequence(b)
		return
	}
	if len(l.utf8) > 0 || b >= utf8.RuneSelf {
		l.multibyte(b)
		return
	}
	switch {
	case b == esc:
		l.state, l.seq = stateEsc, 1
	case b == '\n':
		l.newLine()
	case b == '\r':
		l.overwrite = true
	case b == backspace:
		if len(l.cur) > 0 {
			l.cur = l.cur[:len(l.cur)-1]
			l.changed = true
		}
	case b == '\t':
		for n := tabWidth - len(l.cur)%tabWidth; n > 0; n-- {
			l.put(' ')
		}
	case b < 0x20 || b == del:
		// Another control character: dropped.
	default:
		l.put(rune(b))
	}
}

// multibyte reads a byte of a character of more than one byte. A character that is not valid
// becomes U+FFFD, and the bytes after its first are read again.
func (l *Log) multibyte(b byte) {
	l.utf8 = append(l.utf8, b)
	if !utf8.FullRune(l.utf8) {
		return
	}
	r, size := utf8.DecodeRune(l.utf8)
	rest := append([]byte(nil), l.utf8[size:]...)
	l.utf8 = l.utf8[:0]
	l.put(r)
	for _, b := range rest {
		l.byte(b)
	}
}

// sequence reads a byte of an escape sequence, which is dropped. A sequence that has not ended
// after maxSequence bytes is given up: its bytes are dropped, and the bytes after them are read
// normally.
func (l *Log) sequence(b byte) {
	l.seq++
	if l.seq > maxSequence {
		l.state = stateText
		l.byte(b)
		return
	}
	switch l.state {
	case stateEsc:
		switch b {
		case '[':
			l.state = stateCSI
		case ']':
			l.state = stateOSC
		case 'P', 'X', '^', '_':
			l.state = stateStr
		default:
			l.state = stateText // ESC and one other byte
		}
	case stateCSI:
		switch {
		case b >= 0x20 && b <= 0x3f:
		case b >= 0x40 && b <= 0x7e:
			l.state = stateText
		default:
			// Not a CSI after all: the sequence ends, and the byte is read as text.
			l.state = stateText
			l.byte(b)
		}
	case stateOSC:
		switch b {
		case bel:
			l.state = stateText
		case esc:
			l.state = stateOSCEsc
		}
	case stateOSCEsc:
		if b == '\\' {
			l.state = stateText
		} else {
			l.state = stateOSC
		}
	case stateStr:
		if b == esc {
			l.state = stateStrEsc
		}
	case stateStrEsc:
		if b == '\\' {
			l.state = stateText
		} else {
			l.state = stateStr
		}
	}
}

// dropped says whether a character never reaches a line: a control character (C0, DEL, C1), a
// format character (such as the bidirectional overrides) or a line or paragraph separator.
func dropped(r rune) bool {
	return unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) ||
		unicode.Is(unicode.Cs, r)
}

// put adds a character to the unfinished line.
func (l *Log) put(r rune) {
	if dropped(r) {
		return
	}
	if l.overwrite {
		l.cur = l.cur[:0]
		l.overwrite = false
		l.changed = true
	}
	if len(l.cur) >= l.maxWidth {
		return
	}
	l.cur = append(l.cur, r)
	l.changed = true
}

// newLine finishes the unfinished line.
func (l *Log) newLine() {
	l.lines = append(l.lines, string(l.cur))
	l.cur = l.cur[:0]
	l.overwrite = false
	l.changed = true
	if extra := len(l.lines) - l.maxLines; extra > 0 {
		l.lines = append([]string(nil), l.lines[extra:]...)
		l.first += extra
	}
}

// String is the whole log, the lines joined by new lines: for tests and logs.
func (l *Log) String() string {
	first, end := l.Len()
	return strings.Join(l.Range(first, end), "\n")
}
