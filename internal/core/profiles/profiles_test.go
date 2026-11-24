// SPDX-License-Identifier: GPL-2.0-only

package profiles

import (
	"context"
	"io/fs"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/core/errtest"
	"github.com/rm4n0s/tube/internal/core/profiles/profilestest"
)

func TestAdd(t *testing.T) {
	t.Run("inline files", func(t *testing.T) {
		f := newFixture(t)
		p := f.add("work", profilestest.Options{})
		if !validID(p.ID) {
			t.Errorf("id %q", p.ID)
		}
		if p.Name != "work" || p.File != "work.ovpn" || p.NeedsPassword || p.Broken != "" {
			t.Errorf("profile = %+v", p)
		}
		if want := []Remote{{Network: "udp", Address: "127.0.0.1:1194"}}; !slices.Equal(p.Remotes, want) {
			t.Errorf("remotes = %+v, want %+v", p.Remotes, want)
		}
		if !p.Created.Equal(testTime) || !p.LastUsed.IsZero() {
			t.Errorf("created %v, last used %v", p.Created, p.LastUsed)
		}
		for _, name := range []string{metaFile, filepath.Join(filesDir, "work.ovpn")} {
			if _, err := os.Stat(filepath.Join(f.store.dir, p.ID, name)); err != nil {
				t.Error(err)
			}
		}
		if p.FilesDir != filepath.Join(f.store.dir, p.ID, filesDir) {
			t.Errorf("files folder %s", p.FilesDir)
		}
		// Every folder of the store is 0700 and every file 0600.
		err := filepath.WalkDir(f.store.dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			want := os.FileMode(0o600)
			if d.IsDir() {
				want = 0o700
			}
			if info.Mode().Perm() != want {
				t.Errorf("%s has mode %o, want %o", path, info.Mode().Perm(), want)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Run("files next to it are copied", func(t *testing.T) {
		f := newFixture(t)
		p := f.add("work", profilestest.Options{Separate: true})
		if want := []string{"ca.crt", "client.crt", "client.key"}; !slices.Equal(p.Files, want) {
			t.Errorf("files = %v, want %v", p.Files, want)
		}
		for _, name := range p.Files {
			if _, err := os.Stat(filepath.Join(p.FilesDir, name)); err != nil {
				t.Error(err)
			}
		}
	})
	t.Run("a profile that asks for a password", func(t *testing.T) {
		f := newFixture(t)
		if p := f.add("work", profilestest.Options{Password: true}); !p.NeedsPassword {
			t.Error("NeedsPassword is false")
		}
	})
	t.Run("the original can go away", func(t *testing.T) {
		f := newFixture(t)
		path := f.write("work.ovpn", profilestest.Options{Separate: true})
		p, err := f.store.Add(f.ctx, path, "work")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(filepath.Dir(path)); err != nil {
			t.Fatal(err)
		}
		got, err := f.store.Get(f.ctx, p.ID)
		if err != nil || got.Broken != "" {
			t.Errorf("Get after the original was removed: %+v, %v", got, err)
		}
	})
	t.Run("the original is only read", func(t *testing.T) {
		f := newFixture(t)
		path := f.write("work.ovpn", profilestest.Options{})
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.store.Add(f.ctx, path, "work"); err != nil {
			t.Fatal(err)
		}
		sameBytes(t, path, before)
	})
}

func TestAddNames(t *testing.T) {
	long := strings.Repeat("n", MaxNameLen)
	tests := []struct {
		name      string
		give      string
		want      string
		wantRoute string
	}{
		{name: "a plain name", give: "work", want: "work"},
		{name: "spaces around it", give: "  work  ", want: "work"},
		{name: "empty", give: "", wantRoute: "Store.Add->CleanName.ProfileNameEmpty"},
		{name: "only spaces", give: "   ", wantRoute: "Store.Add->CleanName.ProfileNameEmpty"},
		{name: "64 characters", give: long, want: long},
		{name: "64 characters that take more bytes", give: strings.Repeat("ü", MaxNameLen), want: strings.Repeat("ü", MaxNameLen)},
		{name: "65 characters", give: long + "n", wantRoute: "Store.Add->CleanName.ProfileNameTooLong"},
		{name: "an escape sequence", give: "\x1b[31mred", want: "�[31mred"},
		{name: "a new line", give: "a\nb", want: "a�b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			p, err := f.store.Add(f.ctx, f.write("x.ovpn", profilestest.Options{}), tt.give)
			if tt.wantRoute != "" {
				errtest.Route(t, err, tt.wantRoute)
				if len(f.folders()) != 0 {
					t.Errorf("the store has %v", f.folders())
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if p.Name != tt.want {
				t.Errorf("name = %q, want %q", p.Name, tt.want)
			}
			data, err := os.ReadFile(filepath.Join(f.store.dir, p.ID, metaFile))
			if err != nil {
				t.Fatal(err)
			}
			for _, b := range data {
				if b < 0x20 && b != '\n' {
					t.Errorf("meta.json has the byte %#x", b)
				}
			}
			if strings.Contains(string(data), `\u001b`) {
				t.Error("meta.json has an escape character")
			}
		})
	}
	t.Run("a name that another profile has, in another case", func(t *testing.T) {
		f := newFixture(t)
		f.add("Work", profilestest.Options{})
		_, err := f.store.Add(f.ctx, f.write("x.ovpn", profilestest.Options{}), "wORK")
		meta := errtest.Route(t, err, "Store.Add->Store.nameFree.ProfileNameTaken")
		if meta["name"] != "wORK" {
			t.Errorf("metadata = %v", meta)
		}
		if len(f.folders()) != 1 {
			t.Errorf("the store has %v", f.folders())
		}
	})
}

func TestAddFailures(t *testing.T) {
	t.Run("a path that does not exist", func(t *testing.T) {
		f := newFixture(t)
		_, err := f.store.Add(f.ctx, filepath.Join(f.src, "missing.ovpn"), "x")
		// The folder exists; the parser cannot open the file in it: its own error, untouched.
		if got := ProblemOf(err); got.Kind != ProblemUnreadable {
			t.Errorf("ProblemOf = %+v (err %v)", got, err)
		}
		if len(f.folders()) != 0 {
			t.Errorf("the store has %v", f.folders())
		}
	})
	t.Run("an empty path", func(t *testing.T) {
		f := newFixture(t)
		_, err := f.store.Add(f.ctx, "", "x")
		errtest.Route(t, err, "Store.Add.ProfilePathEmpty")
	})
	t.Run("a syntax error", func(t *testing.T) {
		f := newFixture(t)
		path := f.write("x.ovpn", profilestest.Options{Extra: []string{`remote "127.0.0.2 1194`}})
		_, err := f.store.Add(f.ctx, path, "x")
		got := ProblemOf(err)
		if got.Kind != ProblemSyntax || got.Line == 0 {
			t.Errorf("ProblemOf = %+v (err %v)", got, err)
		}
		if len(f.folders()) != 0 {
			t.Errorf("the store has %v", f.folders())
		}
	})
	t.Run("a cancelled context", func(t *testing.T) {
		f := newFixture(t)
		ctx, cancel := context.WithCancel(f.ctx)
		cancel()
		_, err := f.store.Add(ctx, f.write("x.ovpn", profilestest.Options{}), "x")
		errtest.Route(t, err, "Store.Add.ProfileAddCancelled")
	})
	t.Run("a file goes away before the copy", func(t *testing.T) {
		f := newFixture(t)
		path := f.write("x.ovpn", profilestest.Options{Separate: true})
		f.store.afterLoad = func() {
			if err := os.Remove(filepath.Join(filepath.Dir(path), "client.key")); err != nil {
				t.Fatal(err)
			}
		}
		_, err := f.store.Add(f.ctx, path, "x")
		meta := errtest.Route(t, err, "Store.Add->Store.store.ProfileCopyReadFailed")
		if meta["file"] != "client.key" {
			t.Errorf("metadata = %v", meta)
		}
		if len(f.folders()) != 0 {
			t.Errorf("the store has %v", f.folders())
		}
	})
	t.Run("the folder goes away before the copy", func(t *testing.T) {
		f := newFixture(t)
		path := f.write("x.ovpn", profilestest.Options{})
		f.store.afterLoad = func() {
			if err := os.RemoveAll(filepath.Dir(path)); err != nil {
				t.Fatal(err)
			}
		}
		_, err := f.store.Add(f.ctx, path, "x")
		errtest.Route(t, err, "Store.Add->Store.store.ProfileCopyFolderOpenFailed")
		if len(f.folders()) != 0 {
			t.Errorf("the store has %v", f.folders())
		}
	})
	t.Run("the profile changes before the copy", func(t *testing.T) {
		f := newFixture(t)
		path := f.write("x.ovpn", profilestest.Options{})
		f.store.afterLoad = func() {
			if err := os.WriteFile(path, []byte("client\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		_, err := f.store.Add(f.ctx, path, "x")
		// The copy is parsed again, and fails: the parser's own error.
		if got := ProblemOf(err); got.Kind == ProblemNone {
			t.Errorf("err = %v, want the parser's", err)
		}
		if len(f.folders()) != 0 {
			t.Errorf("the store has %v", f.folders())
		}
	})
}

func TestList(t *testing.T) {
	t.Run("an empty store", func(t *testing.T) {
		f := newFixture(t)
		list, err := f.store.List(f.ctx)
		if err != nil || len(list) != 0 {
			t.Errorf("List = %v, %v", list, err)
		}
	})
	t.Run("sorted by name without case", func(t *testing.T) {
		f := newFixture(t)
		for _, name := range []string{"beta", "Alpha", "gamma"} {
			f.add(name, profilestest.Options{})
		}
		list, err := f.store.List(f.ctx)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, p := range list {
			names = append(names, p.Name)
		}
		if want := []string{"Alpha", "beta", "gamma"}; !slices.Equal(names, want) {
			t.Errorf("names = %v, want %v", names, want)
		}
	})
	t.Run("folders without meta.json", func(t *testing.T) {
		f := newFixture(t)
		f.add("work", profilestest.Options{})
		old := filepath.Join(f.store.dir, "aaaaaaaaaaaaaaaa")
		young := filepath.Join(f.store.dir, "bbbbbbbbbbbbbbbb")
		for dir, age := range map[string]time.Duration{old: 2 * time.Minute, young: 10 * time.Second} {
			if err := os.MkdirAll(filepath.Join(dir, filesDir), 0o700); err != nil {
				t.Fatal(err)
			}
			when := testTime.Add(-age)
			if err := os.Chtimes(dir, when, when); err != nil {
				t.Fatal(err)
			}
		}
		list, err := f.store.List(f.ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(list) != 1 || list[0].Name != "work" {
			t.Errorf("List = %+v, want only work", list)
		}
		if _, err := os.Stat(old); !os.IsNotExist(err) {
			t.Error("the old failed add is still there")
		}
		if _, err := os.Stat(young); err != nil {
			t.Error("the young folder was removed: it may be an add in progress")
		}
	})
	t.Run("what is not a folder of the store is left alone", func(t *testing.T) {
		f := newFixture(t)
		f.add("work", profilestest.Options{})
		if err := os.WriteFile(filepath.Join(f.store.dir, "notes.txt"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(f.store.dir, "Not-An-Id"), 0o700); err != nil {
			t.Fatal(err)
		}
		list, err := f.store.List(f.ctx)
		if err != nil || len(list) != 1 {
			t.Errorf("List = %+v, %v", list, err)
		}
	})
	t.Run("broken profiles", func(t *testing.T) {
		f := newFixture(t)
		garbage := f.add("garbage", profilestest.Options{})
		newer := f.add("newer", profilestest.Options{})
		damaged := f.add("damaged", profilestest.Options{})
		if err := os.WriteFile(filepath.Join(f.store.dir, garbage.ID, metaFile), []byte("{not json"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(f.store.dir, newer.ID, metaFile), []byte(`{"version": 2, "name": "newer"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(damaged.FilesDir, damaged.File), []byte("client\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		list, err := f.store.List(f.ctx)
		if err != nil {
			t.Fatal(err)
		}
		byID := map[string]Profile{}
		for _, p := range list {
			byID[p.ID] = p
		}
		if p := byID[garbage.ID]; p.Name != garbage.ID || p.Broken != "its meta.json cannot be read" {
			t.Errorf("the garbage meta.json: %+v", p)
		}
		if p := byID[newer.ID]; p.Name != newer.ID || p.Broken != "it was made by a newer Tube" {
			t.Errorf("the newer meta.json: %+v", p)
		}
		if p := byID[damaged.ID]; p.Name != "damaged" || !strings.HasPrefix(p.Broken, "its profile cannot be read: ") {
			t.Errorf("the damaged profile: %+v", p)
		}
	})
	t.Run("the store's folder cannot be read", func(t *testing.T) {
		f := newFixture(t)
		if err := os.WriteFile(f.store.dir, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := f.store.List(f.ctx)
		errtest.Route(t, err, "Store.List.ProfileListFailed")
	})
}

func TestGet(t *testing.T) {
	f := newFixture(t)
	p := f.add("work", profilestest.Options{})
	got, err := f.store.Get(f.ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "work" || got.ID != p.ID || !slices.Equal(got.Remotes, p.Remotes) {
		t.Errorf("Get = %+v, want %+v", got, p)
	}
	for _, id := range []string{"", "../x", "abcdef012345678", "abcdef01234567890", "ABCDEF0123456789", "abcdef012345678g", "../../../../etc/"} {
		_, err := f.store.Get(f.ctx, id)
		errtest.Route(t, err, "Store.Get->Store.profileDir.ProfileIDInvalid")
	}
	_, err = f.store.Get(f.ctx, "0123456789abcdef")
	errtest.Route(t, err, "Store.Get->Store.profileDir.ProfileNotFound")
	if !errors.Is(err, ErrNotFound) {
		t.Error("errors.Is(err, ErrNotFound) is false")
	}
	t.Run("a folder that cannot be looked into", func(t *testing.T) {
		notRoot(t)
		chmod(t, filepath.Join(f.store.dir, p.ID), 0)
		_, err := f.store.Get(f.ctx, p.ID)
		errtest.Route(t, err, "Store.Get->Store.profileDir.ProfileStatFailed")
	})
}

func TestChanges(t *testing.T) {
	t.Run("rename", func(t *testing.T) {
		f := newFixture(t)
		work := f.add("work", profilestest.Options{})
		f.add("home", profilestest.Options{})
		if err := f.store.Rename(f.ctx, work.ID, "office"); err != nil {
			t.Fatal(err)
		}
		if err := f.store.Rename(f.ctx, work.ID, "OFFICE"); err != nil {
			t.Errorf("a rename to its own name in another case: %v", err)
		}
		errtest.Route(t, f.store.Rename(f.ctx, work.ID, "Home"), "Store.Rename->Store.nameFree.ProfileNameTaken")
		errtest.Route(t, f.store.Rename(f.ctx, work.ID, ""), "Store.Rename->CleanName.ProfileNameEmpty")
		got, err := f.store.Get(f.ctx, work.ID)
		if err != nil || got.Name != "OFFICE" {
			t.Errorf("Get = %+v, %v", got, err)
		}
	})
	t.Run("user name", func(t *testing.T) {
		f := newFixture(t)
		p := f.add("work", profilestest.Options{Password: true})
		if err := f.store.SetUsername(f.ctx, p.ID, "  ada\x07  "); err != nil {
			t.Fatal(err)
		}
		got, err := f.store.Get(f.ctx, p.ID)
		if err != nil || got.Username != "ada�" {
			t.Errorf("Get = %+v, %v", got, err)
		}
		if err := f.store.SetUsername(f.ctx, p.ID, ""); err != nil {
			t.Fatal(err)
		}
		if got, _ := f.store.Get(f.ctx, p.ID); got.Username != "" {
			t.Errorf("the user name was not forgotten: %q", got.Username)
		}
	})
	t.Run("DNS servers", func(t *testing.T) {
		f := newFixture(t)
		p := f.add("work", profilestest.Options{})
		dns := []netip.Addr{netip.MustParseAddr("10.0.0.53"), netip.MustParseAddr("fd00::53")}
		if err := f.store.SetDNS(f.ctx, p.ID, dns); err != nil {
			t.Fatal(err)
		}
		got, err := f.store.Get(f.ctx, p.ID)
		if err != nil || !slices.Equal(got.DNS, dns) {
			t.Errorf("Get = %+v, %v", got, err)
		}
		five := slices.Repeat([]netip.Addr{netip.MustParseAddr("10.0.0.1")}, 5)
		errtest.Route(t, f.store.SetDNS(f.ctx, p.ID, five), "Store.SetDNS.ProfileTooManyDNS")
		if err := f.store.SetDNS(f.ctx, p.ID, nil); err != nil {
			t.Fatal(err)
		}
		if got, _ := f.store.Get(f.ctx, p.ID); len(got.DNS) != 0 {
			t.Errorf("the DNS servers were not removed: %v", got.DNS)
		}
	})
	t.Run("mark used", func(t *testing.T) {
		f := newFixture(t)
		p := f.add("work", profilestest.Options{})
		f.clock.Advance(time.Hour)
		if err := f.store.MarkUsed(f.ctx, p.ID); err != nil {
			t.Fatal(err)
		}
		got, err := f.store.Get(f.ctx, p.ID)
		if err != nil || !got.LastUsed.Equal(testTime.Add(time.Hour)) {
			t.Errorf("Get = %+v, %v", got, err)
		}
	})
	t.Run("delete", func(t *testing.T) {
		f := newFixture(t)
		p := f.add("work", profilestest.Options{})
		if err := f.store.Delete(f.ctx, p.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(f.store.dir, p.ID)); !os.IsNotExist(err) {
			t.Error("the folder is still there")
		}
		errtest.Route(t, f.store.Delete(f.ctx, p.ID), "Store.Delete->Store.profileDir.ProfileNotFound")
	})
	t.Run("a delete that fails", func(t *testing.T) {
		notRoot(t)
		f := newFixture(t)
		p := f.add("work", profilestest.Options{})
		chmod(t, f.store.dir, 0o500)
		errtest.Route(t, f.store.Delete(f.ctx, p.ID), "Store.Delete.ProfileDeleteFailed")
	})
	t.Run("a profile that is not there", func(t *testing.T) {
		f := newFixture(t)
		const id = "0123456789abcdef"
		errtest.Route(t, f.store.Rename(f.ctx, id, "x"), "Store.update->Store.profileDir.ProfileNotFound")
		errtest.Route(t, f.store.SetUsername(f.ctx, id, "x"), "Store.update->Store.profileDir.ProfileNotFound")
		errtest.Route(t, f.store.SetDNS(f.ctx, id, nil), "Store.update->Store.profileDir.ProfileNotFound")
		errtest.Route(t, f.store.MarkUsed(f.ctx, id), "Store.update->Store.profileDir.ProfileNotFound")
	})
	t.Run("a meta.json that cannot be changed", func(t *testing.T) {
		f := newFixture(t)
		garbage := f.add("garbage", profilestest.Options{})
		newer := f.add("newer", profilestest.Options{})
		if err := os.WriteFile(filepath.Join(f.store.dir, garbage.ID, metaFile), []byte("{not json"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(f.store.dir, newer.ID, metaFile), []byte(`{"version": 2}`), 0o600); err != nil {
			t.Fatal(err)
		}
		errtest.Route(t, f.store.SetUsername(f.ctx, garbage.ID, "x"), "Store.SetUsername->Store.update->readMeta.ProfileMetaInvalid")
		errtest.Route(t, f.store.SetUsername(f.ctx, newer.ID, "x"), "Store.SetUsername->Store.update->readMeta.ProfileMetaNewer")
	})
}

func TestParseDNS(t *testing.T) {
	tests := []struct {
		text      string
		want      []string
		wantRoute string
	}{
		{text: "", want: nil},
		{text: "1.1.1.1", want: []string{"1.1.1.1"}},
		{text: "1.1.1.1, 2606:4700::1111", want: []string{"1.1.1.1", "2606:4700::1111"}},
		{text: "1.1.1.1 8.8.8.8", want: []string{"1.1.1.1", "8.8.8.8"}},
		{text: "1.1.1.1,,8.8.8.8", want: []string{"1.1.1.1", "8.8.8.8"}},
		{text: "dns.example", wantRoute: "ParseDNS.ProfileDNSInvalid"},
		{text: "0.0.0.0", wantRoute: "ParseDNS.ProfileDNSInvalid"},
		{text: "::", wantRoute: "ParseDNS.ProfileDNSInvalid"},
		{text: "fe80::1%eth0", wantRoute: "ParseDNS.ProfileDNSInvalid"},
		{text: "1.1.1.1 1.0.0.1 8.8.8.8 8.8.4.4 9.9.9.9", wantRoute: "ParseDNS.ProfileDNSTooMany"},
	}
	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			got, err := ParseDNS(tt.text)
			if tt.wantRoute != "" {
				errtest.Route(t, err, tt.wantRoute)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var texts []string
			for _, a := range got {
				texts = append(texts, a.String())
			}
			if !slices.Equal(texts, tt.want) {
				t.Errorf("ParseDNS = %v, want %v", texts, tt.want)
			}
		})
	}
}

func TestProblemOf(t *testing.T) {
	pki := profilestest.NewPKI(t)
	tests := []struct {
		name  string
		extra []string
		want  ProblemKind
	}{
		{"an option that is not supported", []string{"comp-lzo"}, ProblemUnsupported},
		{"an option with wrong arguments", []string{"keepalive 10"}, ProblemBadOption},
		{"a file that is not there", []string{"tls-crypt missing.key"}, ProblemFile},
		{"a syntax error", []string{`remote "x`}, ProblemSyntax},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			profilestest.WriteProfile(t, dir, "a.ovpn", pki, profilestest.Options{Extra: tt.extra})
			_, _, err := Load(dir, "a.ovpn")
			got := ProblemOf(err)
			if got.Kind != tt.want || got.Line == 0 {
				t.Errorf("ProblemOf = %+v (err %v), want kind %d with a line", got, err, tt.want)
			}
		})
	}
	t.Run("an option that is missing", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "a.ovpn"), []byte("client\nremote 127.0.0.1 1194\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, _, err := Load(dir, "a.ovpn")
		if got := ProblemOf(err); got.Kind != ProblemMissing || got.Option != "ca" {
			t.Errorf("ProblemOf = %+v (err %v)", got, err)
		}
	})
	t.Run("not the parser's", func(t *testing.T) {
		for _, err := range []error{nil, os.ErrNotExist, errors.New("Other", "other", "option", "x")} {
			if got := ProblemOf(err); got != (Problem{}) {
				t.Errorf("ProblemOf(%v) = %+v", err, got)
			}
		}
	})
	t.Run("the option is made safe", func(t *testing.T) {
		err := errors.New("UnsupportedOption", "x", "option", "evil\x1b[2J", "line", 3, "reason", "a\nb")
		got := ProblemOf(err)
		if got.Option != "evil�[2J" || got.Reason != "a�b" || got.Line != 3 {
			t.Errorf("ProblemOf = %+v", got)
		}
	})
}

func TestSecretsStayOut(t *testing.T) {
	const marker = "TUBE-SECRET-MARKER-7c1f"
	f := newFixture(t)
	path := f.write("x.ovpn", profilestest.Options{Separate: true})
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), "client.key"), []byte(marker+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := f.store.Add(f.ctx, path, "x")
	if err == nil {
		t.Fatal("a key that is not a key was accepted")
	}
	if strings.Contains(err.Error(), marker) {
		t.Errorf("the message has the key: %v", err)
	}
	if appErr, ok := errors.FromError(err); ok {
		for k, v := range appErr.Metadata {
			if s, ok := v.(string); ok && strings.Contains(s, marker) {
				t.Errorf("the metadata %q has the key", k)
			}
		}
	}
}
