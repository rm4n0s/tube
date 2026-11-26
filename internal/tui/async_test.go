// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/app"
	"github.com/rm4n0s/tube/internal/app/apptest"
)

// failing is the fake application with one command that fails as a command of the App does:
// with an error that is made where the command runs, which is the goroutine of the command.
type failing struct {
	*apptest.Fake
	// on is the name of the command that fails; "" lets every command work.
	on string
}

func (f *failing) Snapshot(ctx context.Context) (app.Snapshot, error) {
	if f.on == "Snapshot" {
		return app.Snapshot{}, errors.New("TestCommandFailed", "the snapshot cannot be read")
	}
	return f.Fake.Snapshot(ctx)
}

func (f *failing) AddProfile(ctx context.Context, path, name, dns string, withoutScripts bool) (string, error) {
	if f.on == "AddProfile" {
		return "", errors.New("TestCommandFailed", "the profile cannot be added")
	}
	return f.Fake.AddProfile(ctx, path, name, dns, withoutScripts)
}

func (f *failing) RenameProfile(ctx context.Context, id, name string) error {
	if f.on == "RenameProfile" {
		return errors.New("TestCommandFailed", "the profile cannot be renamed")
	}
	return f.Fake.RenameProfile(ctx, id, name)
}

func (f *failing) SetUsername(ctx context.Context, id, username string) error {
	if f.on == "SetUsername" {
		return errors.New("TestCommandFailed", "the user name cannot be set")
	}
	return f.Fake.SetUsername(ctx, id, username)
}

func (f *failing) SetDNS(ctx context.Context, id, text string) error {
	if f.on == "SetDNS" {
		return errors.New("TestCommandFailed", "the DNS servers cannot be set")
	}
	return f.Fake.SetDNS(ctx, id, text)
}

func (f *failing) DeleteProfile(ctx context.Context, id string) error {
	if f.on == "DeleteProfile" {
		return errors.New("TestCommandFailed", "the profile cannot be deleted")
	}
	return f.Fake.DeleteProfile(ctx, id)
}

func (f *failing) BeginEdit(ctx context.Context, id string) (string, error) {
	if f.on == "BeginEdit" {
		return "", errors.New("TestCommandFailed", "the edit cannot begin")
	}
	return f.Fake.BeginEdit(ctx, id)
}

func (f *failing) EndEdit(ctx context.Context, id, path string) (bool, error) {
	if f.on == "EndEdit" {
		return false, errors.New("TestCommandFailed", "the edit cannot end")
	}
	return f.Fake.EndEdit(ctx, id, path)
}

func (f *failing) ForgetPassword(ctx context.Context, id string) error {
	if f.on == "ForgetPassword" {
		return errors.New("TestCommandFailed", "the password cannot be forgotten")
	}
	return f.Fake.ForgetPassword(ctx, id)
}

func (f *failing) Connect(ctx context.Context, id string, creds *app.Credentials, remember app.Remember) error {
	if f.on == "Connect" {
		return errors.New("TestCommandFailed", "the VPN cannot be connected")
	}
	return f.Fake.Connect(ctx, id, creds, remember)
}

func (f *failing) Start(ctx context.Context) error {
	if f.on == "Start" {
		return errors.New("TestCommandFailed", "the application cannot be started")
	}
	return f.Fake.Start(ctx)
}

func (f *failing) Input(p []byte) error {
	if f.on == "Input" {
		return errors.New("TestCommandFailed", "the input cannot be written")
	}
	return f.Fake.Input(p)
}

func (f *failing) Switch(ctx context.Context, id string, creds *app.Credentials, remember app.Remember) error {
	if f.on == "Switch" {
		return errors.New("TestCommandFailed", "the VPN cannot be switched")
	}
	return f.Fake.Switch(ctx, id, creds, remember)
}

// errOf is the error that a message of a command brings to Update, or nil.
func errOf(msg tea.Msg) error {
	switch msg := msg.(type) {
	case snapshotMsg:
		return msg.err
	case doneMsg:
		return msg.err
	case addedMsg:
		return msg.err
	case deletedMsg:
		return msg.err
	case fieldDoneMsg:
		return msg.err
	case forgotMsg:
		return msg.err
	case editBeganMsg:
		return msg.err
	case editEndedMsg:
		return msg.err
	case connectedMsg:
		return msg.err
	case startedMsg:
		return msg.err
	case switchedMsg:
		return msg.err
	}
	return nil
}

// The program runs every command in a goroutine of its own, and Update, in the goroutine of the
// program, gets the error in a message and shows it. The route of the error goes on from the
// Update that made the command, as if Update had called the App itself (errors.Async), and has
// nothing of the command's goroutine in it (CLAUDE.md "Error handling").
func TestTheErrorOfACommandStartsAtUpdate(t *testing.T) {
	// run is the model on the run screen, as enter on Home shows it.
	run := func(h *harness) {
		h.selectProfile("id-home")
		h.key("enter")
	}
	field := func(key string) func(h *harness) {
		return func(h *harness) {
			h.open("id-work")
			h.key(key)
		}
	}
	editor := func(h *harness) {
		var argvs [][]string
		h.withEditor(doneAtOnce(nil, &argvs))
		h.open("id-work")
	}
	tests := []struct {
		name string
		// on is the command of the App that fails.
		on string
		// before brings the model to where the command is asked for, while every command works.
		before func(h *harness)
		// ask makes Update return the command.
		ask       func(h *harness)
		wantRoute string
	}{
		{
			name: "reading the snapshot", on: "Snapshot",
			ask: func(h *harness) { h.changed() },
			wantRoute: "Model.Update->Model.readSnapshot->Model.readSnapshot.func1->" +
				"failing.Snapshot.TestCommandFailed",
		},
		{
			name: "adding a profile", on: "AddProfile",
			before: func(h *harness) {
				h.key("a")
				h.typeText("/tmp/x.ovpn")
			},
			ask: func(h *harness) { h.key("enter") },
			wantRoute: "Model.Update->Model.key->Model.addKey->Model.addSubmit->Model.addProfile->" +
				"Model.addProfile.func1->failing.AddProfile.TestCommandFailed",
		},
		{
			name: "renaming a profile", on: "RenameProfile", before: field("r"),
			ask: func(h *harness) { h.key("enter") },
			wantRoute: "Model.Update->Model.key->Model.profileKey->Model.fieldKey->" +
				"Model.fieldKey.func1->failing.RenameProfile.TestCommandFailed",
		},
		{
			name: "setting the user name", on: "SetUsername", before: field("u"),
			ask: func(h *harness) { h.key("enter") },
			wantRoute: "Model.Update->Model.key->Model.profileKey->Model.fieldKey->" +
				"Model.fieldKey.func1->failing.SetUsername.TestCommandFailed",
		},
		{
			name: "setting the DNS servers", on: "SetDNS", before: field("n"),
			ask: func(h *harness) { h.key("enter") },
			wantRoute: "Model.Update->Model.key->Model.profileKey->Model.fieldKey->" +
				"Model.fieldKey.func1->failing.SetDNS.TestCommandFailed",
		},
		{
			name: "deleting a profile", on: "DeleteProfile",
			before: func(h *harness) { h.open("id-work") },
			ask:    func(h *harness) { h.keys("d", "y") },
			wantRoute: "Model.Update->Model.key->Model.askDelete.func1->Model.askDelete.func1.1->" +
				"failing.DeleteProfile.TestCommandFailed",
		},
		{
			name: "forgetting the password", on: "ForgetPassword",
			before: func(h *harness) { h.open("id-work") },
			ask:    func(h *harness) { h.keys("p", "y") },
			wantRoute: "Model.Update->Model.key->Model.askForget.func1->Model.askForget.func1.1->" +
				"failing.ForgetPassword.TestCommandFailed",
		},
		{
			name: "beginning an edit", on: "BeginEdit", before: editor,
			ask: func(h *harness) { h.key("t") },
			wantRoute: "Model.Update->Model.key->Model.profileKey->Model.startEdit->" +
				"Model.startEdit.func1->failing.BeginEdit.TestCommandFailed",
		},
		{
			name: "ending an edit", on: "EndEdit", before: editor,
			ask: func(h *harness) { h.key("t") },
			wantRoute: "Model.Update->Model.editorDone->Model.editorDone.func1->" +
				"failing.EndEdit.TestCommandFailed",
		},
		{
			name: "connecting", on: "Connect",
			before: func(h *harness) { h.selectProfile("id-home") },
			ask:    func(h *harness) { h.key("enter") },
			wantRoute: "Model.Update->Model.key->Model.profilesKey->Model.startConnect->" +
				"Model.connect->Model.connect.func1->failing.Connect.TestCommandFailed",
		},
		{
			name: "starting the application", on: "Start",
			before: func(h *harness) { h.selectProfile("id-home") },
			ask:    func(h *harness) { h.key("enter") },
			wantRoute: "Model.Update->Model.connected->Model.start->Model.start.func1->" +
				"failing.Start.TestCommandFailed",
		},
		{
			name: "typing to the application", on: "Input",
			before: func(h *harness) {
				run(h)
				h.key("tab")
			},
			ask: func(h *harness) { h.typeText("x") },
			wantRoute: "Model.Update->Model.key->Model.runKey->Model.outputKey->Model.input->" +
				"Model.input.func1->failing.Input.TestCommandFailed",
		},
		{
			name: "switching the VPN", on: "Switch",
			before: func(h *harness) {
				run(h)
				h.keys("tab", "tab")
				h.selectProfile("id-home")
			},
			ask: func(h *harness) { h.key("enter") },
			wantRoute: "Model.Update->Model.key->Model.runKey->Model.networkKey->" +
				"Model.chooseProfile->Model.switchTo->Model.switchTo.func1->failing.Switch.TestCommandFailed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := apptest.Running()
			fake.Snap.Profiles[2].HasPassword = true
			f := &failing{Fake: fake}
			h := newHarness(t, f, 100, 30)
			if tt.before != nil {
				tt.before(h)
			}
			f.on, h.across = tt.on, true
			tt.ask(h)
			var err error
			for _, msg := range h.seen {
				if err = errOf(msg); err != nil {
					break
				}
			}
			appErr, ok := errors.FromError(err)
			if !ok {
				t.Fatalf("no command brought an *errors.Error: %v, of the messages %T", err, h.seen)
			}
			if !appErr.HasRoute(tt.wantRoute) {
				t.Errorf("unexpected failure path: %s (want %s)", appErr.Route(), tt.wantRoute)
			}
			if route := appErr.Route(); strings.Contains(route, "harness.exec.func1") {
				t.Errorf("the route has the goroutine of the command in it: %s", route)
			}
			h.has("TestCommandFailed")
		})
	}
}
