// SPDX-License-Identifier: GPL-2.0-only

//go:build tube_release

package archtest

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// archiveNames are the names of the files in a .tar.gz.
func archiveNames(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Error(err)
		}
	}()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	r := tar.NewReader(gz)
	var names []string
	for {
		h, err := r.Next()
		if err == io.EOF {
			return names
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, h.Name)
	}
}

func TestReleaseBuild(t *testing.T) {
	root := moduleRoot(t)
	dist := t.TempDir()
	cmd := exec.Command(filepath.Join(root, "scripts", "build-release.sh"), "--version", "9.9.9", "--dist", dist)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build-release.sh: %v\n%s", err, out)
	}
	out := filepath.Join(dist, "9.9.9")
	for _, name := range []string{"tube-9.9.9-linux-amd64.tar.gz", "tube-9.9.9-linux-arm64.tar.gz", "SHA256SUMS"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	check := exec.Command("sha256sum", "-c", "SHA256SUMS")
	check.Dir = out
	if got, err := check.CombinedOutput(); err != nil {
		t.Errorf("sha256sum -c: %v\n%s", err, got)
	}
	amd64 := filepath.Join(out, "tube-9.9.9-linux-amd64.tar.gz")
	if names := archiveNames(t, amd64); !slices.Equal(names, []string{"LICENSE", "README.md", "tube"}) {
		t.Errorf("the archive has %q", names)
	}
	if runtime.GOARCH != "amd64" {
		t.Skip("the amd64 program cannot run on this machine")
	}
	unpacked := t.TempDir()
	untar := exec.Command("tar", "-xzf", amd64, "-C", unpacked)
	if got, err := untar.CombinedOutput(); err != nil {
		t.Fatalf("tar: %v\n%s", err, got)
	}
	version, err := exec.Command(filepath.Join(unpacked, "tube"), "--version").Output()
	if err != nil {
		t.Fatalf("tube --version: %v", err)
	}
	if got := strings.TrimSpace(string(version)); got != "tube 9.9.9" {
		t.Errorf("tube --version printed %q", got)
	}
	// The same version builds the same archives: the times and owners are fixed.
	again := t.TempDir()
	cmd = exec.Command(filepath.Join(root, "scripts", "build-release.sh"), "--version", "9.9.9", "--dist", again)
	cmd.Dir = root
	if got, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build-release.sh again: %v\n%s", err, got)
	}
	first, err := os.ReadFile(filepath.Join(out, "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(filepath.Join(again, "9.9.9", "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Errorf("two builds differ:\n%s\n%s", first, second)
	}
}

func TestReleaseRefusesAMistake(t *testing.T) {
	root := moduleRoot(t)
	for _, args := range [][]string{{"--version"}, {"--nope"}, {"--version", "a b"}} {
		cmd := exec.Command(filepath.Join(root, "scripts", "build-release.sh"), args...)
		cmd.Dir = root
		err := cmd.Run()
		if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != 64 {
			t.Errorf("%q: %v", args, err)
		}
	}
}
