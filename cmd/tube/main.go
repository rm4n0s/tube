// SPDX-License-Identifier: GPL-2.0-only

// Command tube runs one application through a VPN, and only that application
// (docs/SPEC.md §1, §11).
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/rm4n0s/errors"
	"github.com/rm4n0s/tube/internal/app"
	"github.com/rm4n0s/tube/internal/core/sandbox"
	"github.com/rm4n0s/tube/internal/core/storage"
	"github.com/rm4n0s/tube/internal/tui"
	"golang.org/x/sys/unix"
)

const (
	// program is the name of this program, as the messages say it.
	program = "tube"
	// statusOK is the exit status of a run that did what it was asked.
	statusOK = 0
	// statusUsage is the exit status of a mistake in the arguments (docs/SPEC.md §10).
	statusUsage = 2
	// statusFailed is the exit status of a failure of Tube itself (docs/SPEC.md §10).
	statusFailed = sandbox.StatusTubeFailed
	// statusInterrupted is the exit status of a connect that the user cancelled, and of a TUI
	// that the user quit before the application ran.
	statusInterrupted = 130
	// TagNotATerminal is the error of the TUI without a terminal.
	TagNotATerminal = "TubeNotATerminal"
)

func main() {
	// The keeper is this same binary, started again inside the namespace (docs/SPEC.md §8.1).
	if sandbox.IsKeeper() {
		os.Exit(sandbox.KeeperMain())
	}
	os.Exit(run(context.Background(), os.Args[1:], os.LookupEnv, os.Stdin, os.Stdout, os.Stderr))
}

// cli is one run of the command line: where it reads and writes, and how it finds its folders.
type cli struct {
	env    storage.Env
	stdin  *os.File
	stdout *os.File
	stderr io.Writer
	// tty is the terminal that asks for a password.
	tty string
}

// run is the program with its arguments, environment and streams given, so that tests can call
// it. It returns the exit status. Standard output belongs to the application (and to --help and
// --version); every message of Tube is one line on standard error that starts with "tube: ".
func run(ctx context.Context, args []string, env storage.Env, stdin, stdout *os.File, stderr io.Writer) int {
	c := cli{env: env, stdin: stdin, stdout: stdout, stderr: stderr, tty: ttyPath}
	return c.run(ctx, args)
}

// isTerminal says whether the file is a terminal.
func isTerminal(f *os.File) bool {
	_, err := unix.IoctlGetTermios(int(f.Fd()), unix.TCGETS)
	return err == nil
}

// checkTerminal fails unless standard input and output are a terminal, which the TUI needs.
func checkTerminal(stdin, stdout *os.File) error {
	if !isTerminal(stdin) || !isTerminal(stdout) {
		return errors.New(TagNotATerminal, "the TUI needs a terminal; use --profile <name>")
	}
	return nil
}

// say writes one line of Tube on standard error. When even that fails there is nowhere to tell.
func (c cli) say(text string) {
	_, _ = fmt.Fprintln(c.stderr, program+": "+text)
}

// fail tells the user about an error and returns the exit status for it.
func (c cli) fail(err error) int {
	c.say(app.Explain(err))
	return app.ExitCode(err)
}

// run is run, with the cli.
func (c cli) run(ctx context.Context, args []string) int {
	f, err := parseFlags(args)
	if err != nil {
		c.say(app.Explain(err))
		c.say("Try 'tube --help'.")
		return statusUsage
	}
	switch {
	case f.help:
		if _, err := io.WriteString(c.stdout, usage); err != nil {
			return statusFailed // standard output is closed: there is nowhere to tell
		}
		return statusOK
	case f.version:
		if _, err := fmt.Fprintln(c.stdout, program, app.Version()); err != nil {
			return statusFailed
		}
		return statusOK
	}
	// The application is looked for before anything else (docs/SPEC.md §8.5); App.Run finds it
	// again.
	if len(f.target) > 0 {
		if _, err := sandbox.LookPath(f.target[0]); err != nil {
			return c.fail(err)
		}
	}
	if f.profile == "" {
		if err := checkTerminal(c.stdin, c.stdout); err != nil {
			return c.fail(err)
		}
	}
	if err := sandbox.Check(ctx); err != nil {
		return c.fail(err)
	}
	opts := app.Options{Env: c.env, Argv: f.target, Lines: c.stderr}
	if f.quiet {
		opts.Lines = nil
	}
	a, err := app.Open(ctx, opts)
	if err != nil {
		return c.fail(err)
	}
	status := c.runApp(ctx, a, f)
	if err := a.Close(); err != nil {
		c.say(app.Explain(err))
	}
	return status
}

// runTUI runs the TUI; the application runs under it (docs/SPEC.md §11). SIGTERM and SIGHUP
// end the TUI; Finish then stops the application. The status is the application's, or 130 when
// the user quit before one was started (0 without an application).
func (c cli) runTUI(ctx context.Context, a *app.App, f flags) int {
	tuiCtx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGHUP)
	res, err := tui.Run(tuiCtx, a, tui.Options{In: c.stdin, Out: c.stdout})
	stop()
	if res.Started {
		status, ferr := a.Finish()
		if ferr != nil {
			c.say(app.Explain(ferr))
		}
		if err != nil {
			c.say(app.Explain(err))
		}
		return status
	}
	if err != nil {
		return c.fail(err)
	}
	if len(f.target) == 0 {
		return statusOK
	}
	return statusInterrupted
}

// runApp connects the profile of --profile and runs the application, or runs the TUI.
func (c cli) runApp(ctx context.Context, a *app.App, f flags) int {
	if f.profile == "" {
		return c.runTUI(ctx, a, f)
	}
	id, needsPassword, username, err := a.FindProfile(ctx, f.profile)
	if err != nil {
		return c.fail(err)
	}
	var creds *app.Credentials
	if needsPassword {
		got, err := c.credentials(f.authFile, username)
		if err != nil {
			return c.fail(err)
		}
		creds = &got
	}
	// SIGINT, SIGTERM and SIGHUP cancel the connect. A script does not change the profile:
	// the user name is not remembered.
	connectCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	err = a.Connect(connectCtx, id, creds, false)
	interrupted := connectCtx.Err() != nil && ctx.Err() == nil
	stop()
	if err != nil {
		if interrupted {
			return statusInterrupted
		}
		return c.fail(err)
	}
	status, err := c.runWithSignals(ctx, a)
	if err != nil {
		c.say(app.Explain(err))
	}
	return status
}

// credentials reads the user name and the password from the file of --auth-file, or asks for
// them on the terminal.
func (c cli) credentials(authFile, username string) (app.Credentials, error) {
	if authFile != "" {
		return readAuthFile(authFile)
	}
	tty, err := openTerminal(c.tty)
	if err != nil {
		return app.Credentials{}, err
	}
	creds, err := askCredentials(tty, username)
	// The terminal was only read; the credentials are what matters.
	_ = tty.Close()
	return creds, err
}

// runWithSignals runs the application. While it runs, SIGINT and SIGQUIT are caught and dropped
// (the terminal sends them to the application too) and SIGTERM and SIGHUP are passed on to it.
// They are caught, never ignored: an ignored signal stays ignored in the application (trap 13).
func (c cli) runWithSignals(ctx context.Context, a *app.App) (int, error) {
	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGHUP)
	done := make(chan struct{})
	passed := make(chan struct{})
	go func() {
		defer close(passed)
		for {
			select {
			case sig := <-sigs:
				if sig == syscall.SIGTERM || sig == syscall.SIGHUP {
					a.Signal(sig.(syscall.Signal))
				}
			case <-done:
				return
			}
		}
	}()
	status, err := a.Run(ctx)
	signal.Stop(sigs)
	close(done)
	<-passed
	return status, err
}
