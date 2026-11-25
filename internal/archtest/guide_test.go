// SPDX-License-Identifier: GPL-2.0-only

package archtest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rm4n0s/tube/internal/app"
)

// firstWords are the first n words of s, separated by single spaces.
func firstWords(s string, n int) string {
	words := strings.Fields(s)
	return strings.Join(words[:min(n, len(words))], " ")
}

func TestTheGuideExplainsEveryError(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(moduleRoot(t), "docs", "GUIDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	// The guide wraps its lines: a sentence is found with its words on any lines.
	guide := strings.Join(strings.Fields(string(data)), " ")
	sentences := app.Sentences()
	if len(sentences) < 20 {
		t.Fatalf("app.Sentences gives %d sentences", len(sentences))
	}
	for _, s := range sentences {
		if words := firstWords(s, 5); !strings.Contains(guide, words) {
			t.Errorf("docs/GUIDE.md does not have %q (of %q)", words, s)
		}
	}
}
