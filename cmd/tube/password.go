// SPDX-License-Identifier: GPL-2.0-only

package main

import (
	"io"
	"os"
	"strings"
	"syscall"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/app"
	"golang.org/x/sys/unix"
)

// The Tags of reading the credentials.
const (
	TagAuthFileTooOpen     = "TubeAuthFileTooOpen"
	TagAuthFileIncomplete  = "TubeAuthFileIncomplete"
	TagAuthFileUnreadable  = "TubeAuthFileUnreadable"
	TagTerminalFailed      = "TubeTerminalFailed"
	TagTerminalWriteFailed = "TubeTerminalWriteFailed"
	TagTerminalReadFailed  = "TubeTerminalReadFailed"
	TagTerminalLineTooLong = "TubeTerminalLineTooLong"
	TagTerminalClosed      = "TubeTerminalClosed"
	TagNoTerminal          = "TubeNoTerminal"
	maxAuthFile            = 4 << 10
	maxTerminalLine        = 4 << 10
	ttyPath                = "/dev/tty"
	authFileForbiddenModes = 0o077
)

// readAuthFile reads the user name (first line) and the password (second line) of a file that
// only the user can read. A "\r" at the end of a line is dropped.
func readAuthFile(path string) (app.Credentials, error) {
	info, err := os.Stat(path)
	if err == nil && info.Mode().Perm()&authFileForbiddenModes != 0 {
		return app.Credentials{}, errors.New(TagAuthFileTooOpen, "the file of --auth-file can be read by others than you: chmod 600 it", "mode", info.Mode().Perm().String())
	}
	var data []byte
	if err == nil {
		data, err = readAtMost(path, maxAuthFile)
	}
	if err != nil {
		return app.Credentials{}, errors.NewErr(TagAuthFileUnreadable, err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) < 2 {
		return app.Credentials{}, errors.New(TagAuthFileIncomplete, "the file of --auth-file needs the user name on the first line and the password on the second")
	}
	return app.Credentials{Username: strings.TrimSuffix(lines[0], "\r"), Password: strings.TrimSuffix(lines[1], "\r")}, nil
}

// readAtMost reads the first max bytes of a file.
func readAtMost(path string, max int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(f, max))
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return data, err
}

// openTerminal opens the user's terminal (/dev/tty), to ask for what a run is missing.
func openTerminal(path string) (*os.File, error) {
	tty, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, errors.NewErr(TagNoTerminal, err).SetMessage("the profile needs a user name and a password and there is no terminal to ask on; use --auth-file, or --username and --password")
	}
	return tty, nil
}

// askCredentials asks on the terminal for the user name (when username is empty) and for the
// password, which is not echoed.
func askCredentials(tty *os.File, username string) (app.Credentials, error) {
	if username == "" {
		line, err := askLine(tty, userPrompt)
		if err != nil {
			return app.Credentials{}, err
		}
		username = line
	}
	password, err := askPassword(tty)
	if err != nil {
		return app.Credentials{}, err
	}
	return app.Credentials{Username: username, Password: password}, nil
}

// askPassword asks for the password with the echo of the terminal off, and always switches it
// back on. The echo goes off before the prompt is written, so that nothing typed after the
// prompt is shown.
func askPassword(tty *os.File) (string, error) {
	fd := int(tty.Fd())
	saved, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err == nil {
		quiet := *saved
		quiet.Lflag &^= unix.ECHO
		err = unix.IoctlSetTermios(fd, unix.TCSETS, &quiet)
	}
	if err != nil {
		return "", errors.NewErr(TagTerminalFailed, err)
	}
	defer func() {
		// There is nothing more to do when the terminal does not take it back; the new line
		// ends the prompt either way.
		_ = unix.IoctlSetTermios(fd, unix.TCSETS, saved)
		_, _ = tty.WriteString("\n")
	}()
	if err := prompt(tty, "Password: "); err != nil {
		return "", err
	}
	return readLine(tty)
}

// prompt writes the text before what the user types.
func prompt(tty *os.File, text string) error {
	if _, err := tty.WriteString(text); err != nil {
		return errors.NewErr(TagTerminalWriteFailed, err)
	}
	return nil
}

// readLine reads one line, byte by byte, so that nothing after it is taken; the "\n" and a
// "\r" before it are dropped.
func readLine(tty *os.File) (string, error) {
	var line []byte
	b := make([]byte, 1)
	for len(line) < maxTerminalLine {
		n, err := tty.Read(b)
		if n == 1 {
			if b[0] == '\n' {
				return strings.TrimSuffix(string(line), "\r"), nil
			}
			line = append(line, b[0])
			continue
		}
		if err == nil {
			continue
		}
		// A terminal whose other side has gone reads EIO.
		if errors.Is(err, io.EOF) || errors.Is(err, syscall.EIO) {
			return "", errors.New(TagTerminalClosed, "the terminal ended before the line")
		}
		return "", errors.NewErr(TagTerminalReadFailed, err)
	}
	return "", errors.New(TagTerminalLineTooLong, "the line is too long", "max", maxTerminalLine)
}
