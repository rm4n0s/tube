// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"testing"

	"github.com/rm4n0s/tube/internal/app/apptest"
)

func TestQuestionAnswers(t *testing.T) {
	q := question{text: "Delete?", yes: "Delete"}
	tests := []struct {
		key     string
		yes, ok bool
	}{
		{"y", true, true}, {"Y", true, true},
		{"n", false, true}, {"N", false, true}, {"esc", false, true}, {"enter", false, true},
		{"x", false, false}, {"down", false, false},
	}
	for _, tt := range tests {
		if yes, ok := q.answer(tt.key); yes != tt.yes || ok != tt.ok {
			t.Errorf("%s: yes %v ok %v", tt.key, yes, ok)
		}
	}
}

func TestGoldenDeleteQuestion(t *testing.T) {
	h := newHarness(t, apptest.Sample(), 100, 30)
	h.key("d")
	h.golden("delete-question")
	h.key("x") // not an answer: the question stays
	h.has("Delete the profile")
}
