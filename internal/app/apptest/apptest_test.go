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
	if err := f.Connect(ctx, "id-work", &app.Credentials{Username: "ada", Password: "hunter2"}, app.Remember{User: true}); err != nil {
		t.Fatal(err)
	}
	if err := f.Connect(ctx, "id-home", nil, app.Remember{}); err != nil {
		t.Fatal(err)
	}
	want := []string{"RenameProfile(id-work, Office)", "Connect(id-work, ada, ***, keep the user name)", "Connect(id-home, nil, keep nothing)"}
	if got := f.Calls(); !slices.Equal(got, want) {
		t.Errorf("Calls = %q, want %q", got, want)
	}
}

func TestFail(t *testing.T) {
	refused := &errors.Error{Tag: "AuthFailed"}
	f := Sample()
	f.Fail = map[string]error{"Connect": refused}
	if err := f.Connect(context.Background(), "id-work", nil, app.Remember{}); err != refused {
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
		go func() { done <- f.Connect(ctx, "id-home", nil, app.Remember{}) }()
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

func TestRunning(t *testing.T) {
	f := Running()
	rs := f.RunState()
	if !rs.Started || !rs.AppRunning || rs.AppName != "firefox" || rs.ProfileName != "Work" || len(rs.Events) != 4 {
		t.Errorf("the run state is %+v", rs)
	}
	if got := SampleTime.Sub(rs.Since); got != 12*time.Minute+31*time.Second {
		t.Errorf("connected for %v", got)
	}
	if first, end := f.OutputLen(); first != 0 || end != 40 {
		t.Errorf("OutputLen = %d, %d", first, end)
	}
	if got := f.OutputRange(38, 45); len(got) != 2 || got[1] != "firefox: line 40 of the output" {
		t.Errorf("OutputRange = %q", got)
	}
	if got := f.OutputRange(50, 60); got != nil {
		t.Errorf("OutputRange past the end = %q", got)
	}
	if rs := Sample().RunState(); rs.Started {
		t.Error("the sample has a started application")
	}
}

func TestTheRunCalls(t *testing.T) {
	f := Running()
	ctx := context.Background()
	if err := f.Start(ctx); err != nil {
		t.Fatal(err)
	}
	f.Disconnect()
	if err := f.Switch(ctx, "id-work", &app.Credentials{Username: "ada", Password: "hunter2"}, app.Remember{User: true}); err != nil {
		t.Fatal(err)
	}
	if err := f.Switch(ctx, "id-home", nil, app.Remember{}); err != nil {
		t.Fatal(err)
	}
	f.Stop()
	want := []string{"Start()", "Disconnect()", "Switch(id-work, ada, ***, keep the user name)", "Switch(id-home, nil, keep nothing)", "Stop()"}
	if got := f.Calls(); !slices.Equal(got, want) {
		t.Errorf("Calls = %q", got)
	}
	if err := f.Input([]byte("ab")); err != nil {
		t.Fatal(err)
	}
	if err := f.Input([]byte{3}); err != nil {
		t.Fatal(err)
	}
	if got := string(f.Typed()); got != "ab\x03" {
		t.Errorf("Typed = %q", got)
	}
	f.ResizeOutput(100, 30)
	if cols, rows := f.Size(); cols != 100 || rows != 30 {
		t.Errorf("Size = %d, %d", cols, rows)
	}
	f.Fail = map[string]error{"Input": errors.New("AppInputFailed", "no terminal")}
	if err := f.Input([]byte("x")); err == nil {
		t.Error("Input did not fail")
	}
}
