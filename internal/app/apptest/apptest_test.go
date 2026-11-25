// SPDX-License-Identifier: GPL-2.0-only

package apptest

import (
	"context"
	"slices"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/app"
)

func TestSample(t *testing.T) {
	snap, err := Sample().Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Profiles) != 5 || !slices.Equal(snap.Target, []string{"firefox"}) || !snap.Now.Equal(SampleTime) {
		t.Fatalf("the sample is %+v", snap)
	}
	byName := map[string]app.Profile{}
	for _, p := range snap.Profiles {
		byName[p.Name] = p
	}
	if w := byName["Work"]; len(w.Remotes) != 2 || !w.NeedsPassword || w.Username != "ada" || len(w.Ignored) != 2 || len(w.Files) != 3 {
		t.Errorf("Work is %+v", w)
	}
	if !byName["work-backup"].LastUsed.IsZero() || byName["0123456789abcdef"].Broken == "" {
		t.Error("work-backup or the broken profile is wrong")
	}
	long := 0
	for _, p := range snap.Profiles {
		if utf8.RuneCountInString(p.Name) == 64 {
			long++
		}
	}
	if long != 1 {
		t.Errorf("%d profiles have a name of 64 characters", long)
	}
	if empty, _ := Empty().Snapshot(context.Background()); len(empty.Profiles) != 0 || len(empty.Target) != 0 {
		t.Errorf("Empty is %+v", empty)
	}
}

func TestCalls(t *testing.T) {
	f := Sample()
	ctx := context.Background()
	if err := f.RenameProfile(ctx, "id-work", "Office"); err != nil {
		t.Fatal(err)
	}
	if err := f.Connect(ctx, "id-work", &app.Credentials{Username: "ada", Password: "hunter2"}, true); err != nil {
		t.Fatal(err)
	}
	if err := f.Connect(ctx, "id-home", nil, false); err != nil {
		t.Fatal(err)
	}
	want := []string{"RenameProfile(id-work, Office)", "Connect(id-work, ada, ***, true)", "Connect(id-home, nil, false)"}
	if got := f.Calls(); !slices.Equal(got, want) {
		t.Errorf("Calls = %q, want %q", got, want)
	}
}

func TestFail(t *testing.T) {
	refused := &errors.Error{Tag: "AuthFailed"}
	f := Sample()
	f.Fail = map[string]error{"Connect": refused}
	if err := f.Connect(context.Background(), "id-work", nil, false); err != refused {
		t.Errorf("Connect = %v", err)
	}
	if _, err := f.AddProfile(context.Background(), "/p.ovpn", "", false); err != nil {
		t.Errorf("AddProfile = %v", err)
	}
}

func TestBlock(t *testing.T) {
	t.Run("until the channel is closed", func(t *testing.T) {
		gate := make(chan struct{})
		f := Sample()
		f.Block = map[string]chan struct{}{"DeleteProfile": gate}
		done := make(chan error, 1)
		go func() { done <- f.DeleteProfile(context.Background(), "id-home") }()
		select {
		case err := <-done:
			t.Fatalf("DeleteProfile did not wait: %v", err)
		case <-time.After(50 * time.Millisecond):
		}
		close(gate)
		if err := <-done; err != nil {
			t.Errorf("DeleteProfile = %v", err)
		}
	})
	t.Run("until the context ends", func(t *testing.T) {
		f := Sample()
		f.Block = map[string]chan struct{}{"Connect": make(chan struct{})}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- f.Connect(ctx, "id-home", nil, false) }()
		cancel()
		if err := <-done; err != context.Canceled {
			t.Errorf("Connect = %v", err)
		}
	})
}

func TestEmit(t *testing.T) {
	f := Empty()
	calls := 0
	cancel := f.Subscribe(func() { calls++ })
	f.Emit()
	cancel()
	f.Emit()
	if calls != 1 {
		t.Errorf("the subscriber was called %d times", calls)
	}
}
