// SPDX-License-Identifier: GPL-2.0-only

package main

import (
	"context"
	"os"
	"strings"

	"github.com/rm4n0s/tube/internal/app"
)

const (
	// userPrompt, dnsPrompt and the rest are what the terminal shows before an answer
	// (docs/SPEC.md §11.2).
	userPrompt = "User name: "
	dnsPrompt  = "DNS servers (empty: the ones of the VPN): "
	yesNo      = " [Y/n] "
	noYes      = " [y/N] "
	// yesOrNo is what a question answers to something that is neither.
	yesOrNo = "Answer y or n.\n"
)

// plan is what a run with --profile uses beside the profile: the credentials (nil when the
// profile needs none, or uses the remembered ones), the DNS servers of the application when
// dnsSet (else the profile's), and whether they are kept with the profile.
type plan struct {
	creds  *app.Credentials
	dns    string
	dnsSet bool
	save   bool
}

// askLine writes the text and reads the line that answers it.
func askLine(tty *os.File, text string) (string, error) {
	if err := prompt(tty, text); err != nil {
		return "", err
	}
	return readLine(tty)
}

// askYesNo asks a question that "y" and "n" answer, until one of them does; an empty line is
// the answer def.
func askYesNo(tty *os.File, text string, def bool) (bool, error) {
	for {
		answer, err := askLine(tty, text)
		if err != nil {
			return false, err
		}
		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "":
			return def, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}
		if err := prompt(tty, yesOrNo); err != nil {
			return false, err
		}
	}
}

// askDNS asks for the DNS servers until the answer is empty (the ones of the VPN) or check
// takes it; what check refuses is explained on the terminal.
func askDNS(tty *os.File, check func(string) error) (string, error) {
	for {
		answer, err := askLine(tty, dnsPrompt)
		if err != nil {
			return "", err
		}
		answer = strings.TrimSpace(answer)
		if answer == "" {
			return "", nil
		}
		refused := check(answer)
		if refused == nil {
			return answer, nil
		}
		if err := prompt(tty, app.Explain(refused)+"\n"); err != nil {
			return "", err
		}
	}
}

// saveQuestion asks whether to keep what the plan has.
func saveQuestion(pl plan) string {
	switch {
	case pl.creds != nil && pl.dnsSet:
		return "Save the user name, the password and the DNS servers with the profile?"
	case pl.creds != nil:
		return "Save the user name and the password with the profile?"
	}
	return "Save the DNS servers with the profile?"
}

// settle decides what the run uses (docs/SPEC.md §11.2). The credentials are the ones of
// --auth-file; else the ones of --username and --password; else the remembered ones; and what
// is still missing of them is asked on the terminal. The DNS servers are the ones of --dns;
// they are asked for when the profile has none of its own and either was added just now or
// something else is asked anyway. checkDNS takes an answer about them, or says what is wrong
// with it. After a question, the terminal is asked whether to save; --save saves without that
// question, and without --save a run that asked nothing saves nothing.
func (c cli) settle(f flags, found app.Found, checkDNS func(string) error) (plan, error) {
	pl := plan{dns: f.dns, dnsSet: f.dnsGiven}
	needUser, needPassword := false, false
	if found.NeedsPassword {
		switch {
		case f.authFile != "":
			creds, err := readAuthFile(f.authFile)
			if err != nil {
				return plan{}, err
			}
			pl.creds = &creds
		case f.password == "" && found.HasPassword && (f.username == "" || f.username == found.Username):
			// The remembered ones: the app takes them when none are given.
		default:
			creds := app.Credentials{Username: f.username, Password: f.password}
			if creds.Username == "" {
				creds.Username = found.Username
			}
			pl.creds = &creds
			needUser, needPassword = creds.Username == "", creds.Password == ""
		}
	}
	askForDNS := !pl.dnsSet && !found.HasDNS && (found.Added || needUser || needPassword)
	if !needUser && !needPassword && !askForDNS {
		pl.save = f.save
		return pl, nil
	}
	tty, err := openTerminal(c.tty)
	if err != nil {
		if needUser || needPassword {
			return plan{}, err
		}
		// Only the DNS servers would be asked for, and nobody is there to ask: the ones of
		// the VPN.
		pl.save = f.save
		return pl, nil
	}
	// The terminal was only asked; the answers are what matters.
	defer func() { _ = tty.Close() }()
	switch {
	case needPassword:
		// The user name is asked first, when there is none.
		creds, err := askCredentials(tty, pl.creds.Username)
		if err != nil {
			return plan{}, err
		}
		pl.creds = &creds
	case needUser:
		name, err := askLine(tty, userPrompt)
		if err != nil {
			return plan{}, err
		}
		pl.creds.Username = name
	}
	if askForDNS {
		answer, err := askDNS(tty, checkDNS)
		if err != nil {
			return plan{}, err
		}
		pl.dns, pl.dnsSet = answer, answer != ""
	}
	switch {
	case f.save:
		pl.save = true
	case pl.creds != nil || pl.dnsSet:
		if pl.save, err = askYesNo(tty, saveQuestion(pl)+yesNo, true); err != nil {
			return plan{}, err
		}
	}
	return pl, nil
}

// findProfile finds the profile that --profile names, or adds it from the file that it names
// (docs/SPEC.md §11.1). A file that runs scripts is added without them when the user says so
// on the terminal; without a terminal it is not added.
func (c cli) findProfile(ctx context.Context, a *app.App, value string) (app.Found, error) {
	found, err := a.FindOrAddProfile(ctx, value, false)
	options, ok := app.IsRunsScripts(err)
	if !ok {
		return found, err
	}
	tty, terr := openTerminal(c.tty)
	if terr != nil {
		c.say("Run this on a terminal, where Tube asks whether to add the profile without its scripts, or add it in the TUI.")
		return app.Found{}, err
	}
	// The terminal was only asked.
	defer func() { _ = tty.Close() }()
	yes, aerr := askYesNo(tty, "This profile runs scripts ("+options+"). Tube never runs them, and sets the DNS of the application itself. Add it without them?"+noYes, false)
	if aerr != nil {
		return app.Found{}, aerr
	}
	if !yes {
		return app.Found{}, err
	}
	return a.FindOrAddProfile(ctx, value, true)
}
