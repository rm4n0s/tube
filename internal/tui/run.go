// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"context"
	"io"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/app"
	"github.com/rm4n0s/tube/internal/core/clock"
)

// TagRunFailed is the error of a TUI that could not run.
const TagRunFailed = "TuiRunFailed"

// Options say where the TUI runs; nil fields are the terminal, the process's environment and
// the system clock.
type Options struct {
	In    io.Reader
	Out   io.Writer
	Env   []string
	Clock clock.Clock
	// width and height give a size to a program without a terminal: for tests.
	width, height int
}

// Run runs the TUI until the user quits or ctx ends, and returns what it did. A cancelled ctx is
// not an error.
func Run(ctx context.Context, a app.Application, opts Options) (Result, error) {
	clk := opts.Clock
	if clk == nil {
		clk = clock.System()
	}
	popts := []tea.ProgramOption{tea.WithContext(ctx)}
	if opts.In != nil {
		popts = append(popts, tea.WithInput(opts.In))
	}
	if opts.Out != nil {
		popts = append(popts, tea.WithOutput(opts.Out))
	}
	if opts.Env != nil {
		popts = append(popts, tea.WithEnvironment(opts.Env))
	}
	if opts.width > 0 {
		popts = append(popts, tea.WithWindowSize(opts.width, opts.height))
	}
	model := NewModel(ctx, a, clk)
	if opts.Env != nil {
		model.getenv = envGetter(opts.Env)
	}
	p := tea.NewProgram(model, popts...)
	// Send waits until the program runs; the app's goroutine must not wait.
	cancel := a.Subscribe(func() { go p.Send(changedMsg{}) })
	defer cancel()
	ticker := clk.NewTicker(time.Second)
	stop := make(chan struct{})
	ticked := make(chan struct{})
	go func() {
		defer close(ticked)
		for {
			select {
			case <-ticker.C():
				p.Send(tickMsg{})
			case <-stop:
				return
			}
		}
	}()
	final, err := p.Run()
	ticker.Stop()
	close(stop)
	<-ticked
	// The model is returned even when the program was killed: whether the application was
	// started is in it.
	var res Result
	if m, ok := final.(Model); ok {
		res = m.Result()
	}
	if ctx.Err() != nil {
		return res, nil
	}
	if err != nil {
		return res, errors.NewErr(TagRunFailed, err)
	}
	return res, nil
}

// envGetter looks a variable up in an environment of "NAME=value" strings; the last one wins, as
// in os/exec.
func envGetter(env []string) func(string) string {
	return func(name string) string {
		value := ""
		for _, kv := range env {
			if k, v, ok := strings.Cut(kv, "="); ok && k == name {
				value = v
			}
		}
		return value
	}
}
