// SPDX-License-Identifier: GPL-2.0-only

package app

import (
	"context"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/core/profiles"
	"github.com/rm4n0s/tube/internal/core/sanitize"
	"github.com/rm4n0s/tube/internal/core/vpn"
)

// Application is what the TUI uses of the App; apptest has a fake of it.
type Application interface {
	Snapshot(ctx context.Context) (Snapshot, error)
	Subscribe(f func()) (cancel func())
	AddProfile(ctx context.Context, path, name string, withoutScripts bool) (string, error)
	RenameProfile(ctx context.Context, id, name string) error
	SetUsername(ctx context.Context, id, username string) error
	SetDNS(ctx context.Context, id, text string) error
	DeleteProfile(ctx context.Context, id string) error
	BeginEdit(ctx context.Context, id string) (string, error)
	EndEdit(ctx context.Context, id, path string) (bool, error)
	ForgetPassword(ctx context.Context, id string) error
	Connect(ctx context.Context, id string, creds *Credentials, remember Remember) error
	Start(ctx context.Context) error
	RunState() RunState
	OutputLen() (first, end int)
	OutputRange(from, to int) []string
	Input(p []byte) error
	ResizeOutput(cols, rows int)
	Disconnect()
	Switch(ctx context.Context, id string, creds *Credentials, remember Remember) error
	Stop()
}

var _ Application = (*App)(nil)

// maxOptions is the length of the list of script options in a question.
const maxOptions = 200

// IsRunsScripts says whether the error is the one of a profile that runs scripts, added without
// disabling them, and which options they are (safe to draw).
func IsRunsScripts(err error) (options string, ok bool) {
	appErr, ok := errors.FromError(err)
	if !ok || appErr.Tag != profiles.TagProfileRunsScripts {
		return "", false
	}
	options, _ = appErr.Metadata["options"].(string)
	return sanitize.Display(options, maxOptions), true
}

// IsAuthFailed says whether the error is the server's refusal of the user name or the password.
func IsAuthFailed(err error) bool {
	return vpn.KindOf(err) == vpn.KindAuthFailed
}
