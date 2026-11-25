// SPDX-License-Identifier: GPL-2.0-only

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rm4n0s/tube/internal/app"
	"github.com/rm4n0s/tube/internal/core/errtest"
	"github.com/rm4n0s/tube/internal/core/pty"
	"golang.org/x/sys/unix"
)

// authFile writes an auth file of the test with the mode.
func authFile(t *testing.T, text string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "auth")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadAuthFile(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		want  app.Credentials
		route string
	}{
		{"good", authFile(t, "alice\nsecret\n", 0o600), app.Credentials{Username: "alice", Password: "secret"}, ""},
		{"no new line at the end", authFile(t, "alice\nsecret", 0o400), app.Credentials{Username: "alice", Password: "secret"}, ""},
		{"Windows line ends", authFile(t, "alice\r\nsecret\r\n", 0o600), app.Credentials{Username: "alice", Password: "secret"}, ""},
		{"one line", authFile(t, "alice\n", 0o600), app.Credentials{}, "readAuthFile.TubeAuthFileIncomplete"},
		{"a missing file", filepath.Join(t.TempDir(), "missing"), app.Credentials{}, "readAuthFile.TubeAuthFileUnreadable"},
		{"readable by others", authFile(t, "alice\nsecret\n", 0o644), app.Credentials{}, "readAuthFile.TubeAuthFileTooOpen"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readAuthFile(tt.path)
			if tt.route != "" {
				errtest.Route(t, err, tt.route)
				if strings.Contains(err.Error(), "secret") {
					t.Errorf("the error has the password: %v", err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("readAuthFile = %+v, %v", got, err)
			}
		})
	}
}

// terminal is a pseudo-terminal whose sides are closed when the test ends.
func terminal(t *testing.T) (master, slave *os.File) {
	t.Helper()
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// A test may have closed one already.
		_ = master.Close()
		_ = slave.Close()
	})
	return master, slave
}

// user plays the user at the master side of the terminal: for each prompt it waits until the
// terminal shows it, then types the answer. It returns all that the terminal showed, when the
// terminal has shown nothing more for 300 ms after the last answer, or when the master side is
// closed.
func user(t *testing.T, master *os.File, steps ...[2]string) <-chan string {
	t.Helper()
	out := make(chan string, 1)
	go func() {
		var b strings.Builder
		buf := make([]byte, 256)
		read := func(d time.Duration) error {
			if err := master.SetReadDeadline(time.Now().Add(d)); err != nil {
				return err
			}
			n, err := master.Read(buf)
			b.Write(buf[:n])
			return err
		}
		for _, step := range steps {
			for !strings.Contains(b.String(), step[0]) {
				if err := read(5 * time.Second); err != nil {
					out <- b.String()
					return
				}
			}
			if step[1] == "" { // the end: the user closes the terminal
				// The test sees what it does through the error of askCredentials.
				_ = master.Close()
				out <- b.String()
				return
			}
			if _, err := master.WriteString(step[1]); err != nil {
				out <- b.String()
				return
			}
		}
		for read(300*time.Millisecond) == nil {
		}
		out <- b.String()
	}()
	return out
}

func TestAskCredentials(t *testing.T) {
	t.Run("the user name and the password", func(t *testing.T) {
		master, slave := terminal(t)
		shown := user(t, master, [2]string{"User name: ", "alice\n"}, [2]string{"Password: ", "secret\n"})
		got, err := askCredentials(slave, "")
		if err != nil || got != (app.Credentials{Username: "alice", Password: "secret"}) {
			t.Fatalf("askCredentials = %+v, %v", got, err)
		}
		screen := <-shown
		if !strings.Contains(screen, "User name: ") || !strings.Contains(screen, "Password: ") || strings.Contains(screen, "secret") {
			t.Errorf("the terminal showed %q", screen)
		}
		termios, err := unix.IoctlGetTermios(int(slave.Fd()), unix.TCGETS)
		if err != nil || termios.Lflag&unix.ECHO == 0 {
			t.Errorf("the echo is not on again: %v", err)
		}
	})
	t.Run("only the password", func(t *testing.T) {
		master, slave := terminal(t)
		shown := user(t, master, [2]string{"Password: ", "secret\n"})
		got, err := askCredentials(slave, "bob")
		if err != nil || got != (app.Credentials{Username: "bob", Password: "secret"}) {
			t.Fatalf("askCredentials = %+v, %v", got, err)
		}
		if screen := <-shown; strings.Contains(screen, "User name") || strings.Contains(screen, "secret") {
			t.Errorf("the terminal showed %q", screen)
		}
	})
	t.Run("the terminal ends before the line", func(t *testing.T) {
		master, slave := terminal(t)
		shown := user(t, master, [2]string{"User name: ", "ali"}, [2]string{"ali", ""})
		_, err := askCredentials(slave, "")
		errtest.Route(t, err, "readLine.TubeTerminalClosed")
		<-shown
	})
}

func TestTerminalFailures(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file")
	if err := os.WriteFile(path, []byte(strings.Repeat("a", maxTerminalLine+10)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Run("a line that is too long", func(t *testing.T) {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }() // only read
		_, err = readLine(f)
		errtest.Route(t, err, "readLine.TubeTerminalLineTooLong")
	})
	t.Run("a terminal that cannot be written", func(t *testing.T) {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }() // only read
		_, err = askCredentials(f, "")
		errtest.Route(t, err, "prompt.TubeTerminalWriteFailed")
	})
	t.Run("not a terminal", func(t *testing.T) {
		f, err := os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }() // the test's own file
		_, err = askCredentials(f, "alice")
		errtest.Route(t, err, "askPassword.TubeTerminalFailed")
	})
	t.Run("a read that fails", func(t *testing.T) {
		f, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }() // the test's own file
		_, err = readLine(f)
		errtest.Route(t, err, "readLine.TubeTerminalReadFailed")
	})
	t.Run("no terminal", func(t *testing.T) {
		_, err := openTerminal(filepath.Join(dir, "no-tty"))
		errtest.Route(t, err, "openTerminal.TubeNoTerminal")
	})
}
