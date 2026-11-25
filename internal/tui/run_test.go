// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/app"
	"github.com/rm4n0s/tube/internal/app/apptest"
)

// syncBuffer is a buffer that the program writes while the test reads it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// runResult is what Run returned.
type runResult struct {
	res Result
	err error
}

// runAsync runs the TUI in a goroutine.
func runAsync(ctx context.Context, a app.Application, opts Options) <-chan runResult {
	done := make(chan runResult, 1)
	go func() {
		res, err := Run(ctx, a, opts)
		done <- runResult{res, err}
	}()
	return done
}

// waitRun waits for the end of Run.
func waitRun(t *testing.T, done <-chan runResult, d time.Duration) runResult {
	t.Helper()
	select {
	case r := <-done:
		return r
	case <-time.After(d):
		t.Fatal("Run did not return")
		return runResult{}
	}
}

func TestRunWritesOnlyToItsWriter(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var out syncBuffer
	r := waitRun(t, runAsync(ctx, apptest.Sample(), Options{In: strings.NewReader("q"), Out: &out, Env: []string{"TERM=xterm"}, width: 100, height: 30}), 10*time.Second)
	if r.err != nil || r.res.Run {
		t.Errorf("Run = %+v, %v", r.res, r.err)
	}
	if out.String() == "" {
		t.Error("nothing was written")
	}
}

func TestRunEndsWhenTheContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	in, w := io.Pipe()
	defer func() { _ = w.Close() }() // only the end of the input
	done := runAsync(ctx, apptest.Sample(), Options{In: in, Out: io.Discard, Env: []string{"TERM=xterm"}, width: 100, height: 30})
	time.Sleep(100 * time.Millisecond)
	cancel()
	if r := waitRun(t, done, 5*time.Second); r.err != nil {
		t.Errorf("a cancelled Run returned %v", r.err)
	}
}

func TestRunSendsTheAppsChanges(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	f := apptest.Sample()
	in, w := io.Pipe()
	var out syncBuffer
	done := runAsync(ctx, f, Options{In: in, Out: &out, Env: []string{"TERM=xterm"}, width: 100, height: 30})
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(ansi.Strip(out.String()), "work-backup") {
		if time.Now().After(deadline) {
			t.Fatalf("the list was not drawn:\n%s", out.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	f.Snap.Profiles = append(f.Snap.Profiles, app.Profile{ID: "id-zz", Name: "Zz-new-profile"})
	f.Emit()
	for !strings.Contains(ansi.Strip(out.String()), "Zz-new-profile") {
		if time.Now().After(deadline) {
			t.Fatalf("the change was not drawn:\n%s", ansi.Strip(out.String()))
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := w.Write([]byte("q")); err != nil {
		t.Fatal(err)
	}
	if r := waitRun(t, done, 5*time.Second); r.err != nil {
		t.Errorf("Run = %v", r.err)
	}
}

func TestRunFails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	in := iotest.ErrReader(io.ErrClosedPipe)
	r := waitRun(t, runAsync(ctx, apptest.Sample(), Options{In: in, Out: io.Discard, Env: []string{"TERM=xterm"}, width: 100, height: 30}), 10*time.Second)
	appErr, ok := errors.FromError(r.err)
	if !ok || !appErr.HasRoute("Run.TuiRunFailed") {
		t.Errorf("Run = %v, want the route Run.TuiRunFailed", r.err)
	}
}
