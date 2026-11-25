// SPDX-License-Identifier: GPL-2.0-only

package app

import (
	"context"
	"encoding/json"
	"path/filepath"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/core/sanitize"
	"github.com/rm4n0s/tube/internal/core/storage"
)

// The file of what Tube remembers between two runs (docs/SPEC.md §5).
const (
	stateFile    = "state.json"
	stateVersion = 1
	// maxStateBytes is the size of the file, at most, and maxFolderLen the length of a folder
	// in it.
	maxStateBytes = 64 << 10
	maxFolderLen  = 4096
)

// state is the content of the state file.
type state struct {
	Version int `json:"version"`
	// AddFolder is the folder of the profile file that was added last.
	AddFolder string `json:"add_folder"`
}

// statePath is the path of the state file.
func (a *App) statePath() string { return filepath.Join(a.dirs.State, stateFile) }

// loadState reads what Tube remembers. A file that is missing, cannot be read, has a version
// that Tube does not know, or names a folder that is not a plain absolute path, remembers
// nothing: the state is only a convenience, so it never stops Tube.
func (a *App) loadState(ctx context.Context) {
	data, err := storage.ReadFile(ctx, a.statePath(), maxStateBytes)
	if err != nil {
		if appErr, ok := errors.FromError(err); !ok || appErr.Tag != storage.TagFileNotExist {
			a.log.Warn("the state file cannot be read", "err", err)
		}
		return
	}
	var st state
	if err := json.Unmarshal(data, &st); err != nil {
		a.log.Warn("the state file cannot be understood", "err", err.Error())
		return
	}
	if st.Version != stateVersion || !plainFolder(st.AddFolder) {
		return
	}
	a.stateMu.Lock()
	a.addFolder = st.AddFolder
	a.stateMu.Unlock()
}

// plainFolder says whether dir can be remembered: an absolute, clean path of at most
// maxFolderLen bytes with nothing in it that is not safe to draw.
func plainFolder(dir string) bool {
	return dir != "" && len(dir) <= maxFolderLen && filepath.IsAbs(dir) && filepath.Clean(dir) == dir &&
		sanitize.Display(dir, maxFolderLen) == dir
}

// lastAddFolder is the folder of the last add, or "".
func (a *App) lastAddFolder() string {
	a.stateMu.Lock()
	defer a.stateMu.Unlock()
	return a.addFolder
}

// rememberAddFolder remembers the folder of the profile file at path, which was added, for the
// next add, in memory and in the state file. A failure is logged: the profile was added all the
// same.
func (a *App) rememberAddFolder(ctx context.Context, path string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		a.log.Warn("the folder of the added profile is not remembered", "err", err.Error())
		return
	}
	dir := filepath.Dir(abs)
	if !plainFolder(dir) {
		return
	}
	a.stateMu.Lock()
	a.addFolder = dir
	a.stateMu.Unlock()
	data, err := json.Marshal(state{Version: stateVersion, AddFolder: dir})
	if err != nil {
		a.log.Warn("the folder of the added profile is not remembered", "err", err.Error())
		return
	}
	if err := storage.WriteFileAtomic(ctx, a.statePath(), append(data, '\n')); err != nil {
		a.log.Warn("the folder of the added profile is not remembered", "err", err)
	}
}
