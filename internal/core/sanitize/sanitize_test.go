// SPDX-License-Identifier: GPL-2.0-only

package sanitize

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

func TestDisplay(t *testing.T) {
	tests := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"plain text", "alice", 64, "alice"},
		{"empty", "", 64, ""},
		{"letters of other scripts", "Ελένη-李雷_ñ", 64, "Ελένη-李雷_ñ"},
		{"a space stays", "ada lovelace", 64, "ada lovelace"},
		{"combining marks stay", "é", 64, "é"},
		{"an emoji stays", "root🙂", 64, "root🙂"},

		{"an escape sequence", "\x1b[2J\x1b[31mroot", 64, "\uFFFD[2J\uFFFD[31mroot"},
		{"an OSC title sequence", "\x1b]0;pwned\x07", 64, "\uFFFD]0;pwned\uFFFD"},
		{"newlines and tabs", "a\nb\rc\td", 64, "a\uFFFDb\uFFFDc\uFFFDd"},
		{"NUL and DEL", "a\x00b\x7fc", 64, "a\uFFFDb\uFFFDc"},
		{"C1 controls", "a\u0085b\u009bc", 64, "a\uFFFDb\uFFFDc"},
		{"a right-to-left override", "user\u202Etxt.exe", 64, "user\uFFFDtxt.exe"},
		{"bidirectional isolates", "\u2066a\u2069", 64, "\uFFFDa\uFFFD"},
		{"zero-width characters", "ro\u200Bot\u200D\uFEFF", 64, "ro\uFFFDot\uFFFD\uFFFD"},
		{"a soft hyphen", "ro\u00ADot", 64, "ro\uFFFDot"},
		{"line and paragraph separators", "a\u2028b\u2029c", 64, "a\uFFFDb\uFFFDc"},
		{"a tag character", "a\U000E0041", 64, "a\uFFFD"},
		{"invalid UTF-8, each bad byte", "a\xff\xfeb", 64, "a\uFFFD\uFFFDb"},
		{"a cut-off multi-byte character", "a\xe2\x82", 64, "a\uFFFD\uFFFD"},
		{"a replacement character of the input stays one", "a\uFFFDb", 64, "a\uFFFDb"},

		{"exactly the limit", "abcde", 5, "abcde"},
		{"one over the limit", "abcdef", 5, "abcd…"},
		{"far over the limit", strings.Repeat("x", 10000), 8, "xxxxxxx…"},
		{"the limit counts characters, not bytes", "ααααα", 5, "ααααα"},
		{"the cut is at a character", "αααααα", 5, "αααα…"},
		{"a limit of one", "abc", 1, "…"},
		{"a limit of one and one character", "a", 1, "a"},
		{"a limit of zero", "abc", 0, ""},
		{"a negative limit", "abc", -3, ""},
		{"unsafe characters count toward the limit", "\x1b\x1b\x1b\x1b", 3, "\uFFFD\uFFFD…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Display(tt.in, tt.max)
			if got != tt.want {
				t.Errorf("Display(%q, %d) = %q, want %q", tt.in, tt.max, got, tt.want)
			}
		})
	}
}

// TestDisplayIsAlwaysSafe checks the promise of Display on every byte value and on a sweep of
// the code points, alone and in the middle of text.
func TestDisplayIsAlwaysSafe(t *testing.T) {
	check := func(in string) {
		t.Helper()
		got := Display(in, 16)
		if !utf8.ValidString(got) {
			t.Fatalf("Display(%q) = %q is not valid UTF-8", in, got)
		}
		if n := utf8.RuneCountInString(got); n > 16 {
			t.Fatalf("Display(%q) has %d characters", in, n)
		}
		for _, r := range got {
			if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029' {
				t.Fatalf("Display(%q) = %q lets U+%04X through", in, got, r)
			}
		}
	}
	for b := 0; b < 256; b++ {
		check(string([]byte{byte(b)}))
		check("ab" + string([]byte{byte(b)}) + "cd")
		check(string([]byte{0xe2, byte(b), 0x80}))
	}
	for r := rune(0); r <= unicode.MaxRune; r += 7 {
		check(string(r))
		check("x" + string(r) + "y")
	}
	for r := rune(0); r < 0x3000; r++ {
		check(string(r))
	}
}

func TestDisplayIsIdempotent(t *testing.T) {
	for _, in := range []string{"alice", "\x1b[31mroot", "a\xffb", strings.Repeat("é", 100), "user\u202Egnp.exe"} {
		once := Display(in, 20)
		if twice := Display(once, 20); twice != once {
			t.Errorf("Display(Display(%q)) = %q, want %q", in, twice, once)
		}
	}
}
