# CLAUDE.md

Guidance for Claude Code when working in this repository (Go, hosted on GitHub). The development process, the error handling and the way the TUI is built and tested are those of [Cave](https://github.com/rm4n0s/cave)'s `CLAUDE.md` (the user, 2026-11-25: "first use the CLAUDE.md of the \"cave\" project from my repos that is using TUI"), adapted to this project.

## Project
Tube runs one application through a VPN, and only that application: `tube firefox` or `tube ping -c 3 example.com` opens a terminal UI where the user keeps OpenVPN profiles (`.ovpn`: add, look at, change, delete) and selects one; Tube connects to it and starts the application so that all of its network goes through the VPN, and nothing else of the machine does (the user, 2026-11-25). The TUI stays open while the application runs, with tabs for the connection and its traffic, for the application's output, and for switching to another profile or disconnecting (the user, 2026-11-25), and it closes by itself when the application has ended (the user, 2026-11-25: "when firefox or other application is closed, then close the tube too"); `tube --profile <name> <application>` runs it without a TUI, on the user's terminal. The OpenVPN client is [gopenvpn-conn](https://github.com/rm4n0s/gopenvpn-conn), the user's library; the TUI is built on Bubble Tea and Lip Gloss. It needs no root: the application runs in a private user and network namespace whose only way out is a TUN device that Tube feeds from the VPN connection, so nothing of the machine's network configuration changes. Tube is for Linux only (the user, 2026-11-25: "a CLI tool for linux"). The program is `tube`, as the repository (the user, 2026-11-25).

**`docs/SPEC.md` is the source of truth.** Read the sections that cover a task before starting it. If the code and the spec disagree, or a task needs a decision the spec doesn't make, ask instead of guessing. When a decision changes, update the spec on the same branch.

## License
This repository is **GPL-2.0-only** (the user, 2026-11-25), because gopenvpn-conn is: a program that uses it and is distributed must be under the GPL-2.0 as a whole. `LICENSE` is the text of the GPL version 2.
- Every Go file starts with `// SPDX-License-Identifier: GPL-2.0-only`
- A new dependency needs a license that can be combined with GPL-2.0-only (MIT, BSD, ISC). Apache-2.0 cannot, without an exception: ask first. `vpnnet` of gopenvpn-conn brings gVisor (Apache-2.0) and is not used here
- "OpenVPN" is a trademark of OpenVPN Inc: the documentation says that Tube connects to OpenVPN servers, never that it is OpenVPN or is endorsed by it

## Commands
- Build: `go build ./...`. Tube is cgo-free; `CGO_ENABLED=0 go build ./...` must always work
- Build the program: `go build -o tube ./cmd/tube`, then `./tube firefox`
- Test all: `go test -race ./...` (the race detector needs cgo, so this one needs a C compiler)
- Test the cgo-free path: `CGO_ENABLED=0 go test ./...`
- Test one package: `go test ./internal/core/profiles -run TestName -v`
- Tests against a real OpenVPN server: `scripts/vpn-ns.sh -- go test -race -p 1 -tags tube_server ./...` (a private user and network namespace, no root; the tests start and stop `openvpn` themselves, on the loopback of that namespace; `-p 1` because the servers of two packages would share the namespace's network)
- Update golden files: `go test ./internal/tui -run Golden -update-goldens`, then review every changed file (the golden files of the TUI are text)
- All the checks of "Definition of done" in one command: `scripts/checks.sh`
- Heavy commands (full test runs, lint, the tests against the real server): through `scripts/heavy.sh`, see "Memory (OOM)"; `scripts/checks.sh` does it itself
- Vet: `go vet ./...`
- Lint: `golangci-lint run`
- Format: `gofmt -w cmd internal`
- Run it: `go run ./cmd/tube <application> [arguments]` (it uses your real profiles)
- Run it isolated from your profiles: `scripts/isolated.sh [--config <fixture dir>] -- go run ./cmd/tube <application> [arguments]`
- Make a proof of the TUI: `scripts/tui-proof.sh <out.txt> ...` (docs/IMPLEMENTATION.md "Proof methods")
- After changing dependencies: `go mod tidy`

A script that a step of `docs/IMPLEMENTATION.md` adds does not exist before that step.

## Definition of done
Before reporting a task as finished, run `gofmt` (on `cmd` and `internal`), `go vet ./...`, `golangci-lint run` and `go test -race ./...`, and fix anything they report. Say so explicitly if any step was skipped or failed. Also:
- `CGO_ENABLED=0 go build ./...` and `CGO_ENABLED=0 go test ./...` pass (proves Tube stays cgo-free)
- From the step that adds them: `scripts/vpn-ns.sh -- go test -race -p 1 -tags tube_server ./...` passes
- `scripts/error-tags.sh --untested` prints nothing (every error Tag has a test that asserts its route; `docs/ERROR-TAGS-UNTESTED.md` is the only exception list)
- Every new exported identifier has a doc comment, and every Go file has its SPDX line
- Every changed golden file has been looked at, not just regenerated
- `pgrep -a openvpn` and `pgrep -a tube` show nothing left of the task

`scripts/checks.sh` runs all of them in this order and stops at the first that fails.

## Layout
- `cmd/tube/` is the `main` package
- `internal/core/...` is everything that is not UI: storage, the profiles, the VPN connection, the namespace and its TUN device, the application that is run, its pseudo-terminal and the log of its output. It never imports Bubble Tea, Lip Gloss or `internal/tui`, so it is tested without a UI
- `internal/app/` is the application behind the TUI: it owns the core's objects and gives the TUI snapshots, commands and change events. It imports the core and no UI library
- `internal/tui/...` is the terminal UI on Bubble Tea. It imports `internal/app`, and nothing imports it but `cmd/tube`
- `internal/archtest/` checks the package boundaries and forbidden imports, and holds the error-tag tool
- `docs/SPEC.md` is the specification; `docs/IMPLEMENTATION.md` is the step-by-step plan; `docs/IMPLEMENTED.md` logs finished steps; `docs/implemented-proof-images/` holds each step's proofs
- `scripts/` holds helper scripts
- Tests live next to the code (`foo_test.go`); golden files live in `testdata/goldens/` of their package
- The exact package list is defined in `docs/SPEC.md`

## Go conventions
- Never ignore a returned error; handle it or return it
- Pass `context.Context` as the first parameter of anything that does I/O (files, processes, network). Name it `ctx`
- Tests are table-driven
- No global mutable state and no singletons. Package-level read-only values are fine and are never reassigned
- Prefer the standard library. Allowed dependencies: `github.com/rm4n0s/gopenvpn-conn` (v1.0.1 or newer; the packages `openvpn` and `ovpn`, not `vpnnet`), `github.com/rm4n0s/errors`, `golang.org/x/sys`, `charm.land/bubbletea/v2` and `charm.land/lipgloss/v2` (only under `internal/tui` and `cmd/tube`), and what these bring in. A step that first imports one of them adds it with `go get <module>@<version>`. Ask before adding any other
- **No cgo**: never write `import "C"`. If a C library is ever needed, ask first
- Linux only: code that needs Linux (namespaces, TUN, netlink) is in `_linux.go` files or in packages that only build for Linux, and nothing is written for another system until the user asks
- Only Bubble Tea's `Update` touches the TUI's model. Other goroutines (the VPN connection, the application that runs, timers) hand work over through a `tea.Msg`
- Long-running work (connecting, reading a file, waiting for the application) never blocks the UI, and takes a `ctx` so it can be cancelled
- Every goroutine and every process has an owner that stops it: when Tube returns, the VPN connection is closed, the namespace is gone and no child is left
- When gopenvpn-conn is missing something or has a bug, work around it in Tube without stopping, mark the workaround with a `// gopenvpn-conn:` comment that says what it works around and what the library should provide instead, and list it in `docs/GOPENVPN-CONN-ISSUES.md` (create the file with the first entry; mention new entries in the report at the end of the task), so that it can be fixed in the library later
- Read the source of a library in the Go module cache when its behavior is unclear (`go list -m -f '{{.Dir}}' github.com/rm4n0s/gopenvpn-conn`); gopenvpn-conn's `README.md` and `docs/SPEC.md` describe its use and design

## Error handling
Use `github.com/rm4n0s/errors` for all errors. Never use `fmt.Errorf`, and never import the standard library `errors` package: this package re-exports `errors.Is`, `errors.As` and `errors.Unwrap`, so a single import covers everything. Read `AI_GUIDE.md` of github.com/rm4n0s/errors (in the Go module cache: `go list -m -f '{{.Dir}}' github.com/rm4n0s/errors`) before writing error code.

**Creating errors**
- New failure with a fixed message: `errors.New("Tag", "message", "key", value, ...)` (all metadata in this one call, never `New(...).SetMetadata(...)`)
  - Tag: short PascalCase name of the failure, e.g. `ProfileReadFailed`
  - Trailing arguments are key/value metadata pairs, e.g. `"path", path`
- New failure with a formatted message: `errors.NewErrf("Tag", "format %s", arg)`
- Wrapping an error that is NOT already an `*errors.Error` (stdlib, `x/sys` or Bubble Tea errors that are not from this package): `errors.NewErr("Tag", err)`. Add metadata with one `.SetMetadata("key", value, "key2", value2)` call (a later call merges into the metadata, it does not replace it, but use one call unless the data only becomes available later)
- Every `New`, `NewErr` and `NewErrf` call captures the stack at that point
- Construct the error in the function that detects the failure, never in a helper that only builds it (`errNoProfile()`, `fail(tag, msg)`): the route is the stack at the constructor, so a helper puts its own name in every route. The tag is a string literal at the call
- Sentinels are struct literals, `&errors.Error{Tag: "X"}`, never a package-level `errors.New`: its stack is `init`, so every route through it is wrong
- Never put a secret in a message or in metadata: no private key, password, token or static key, and no line of a profile (it may be one of them)

**Propagating errors**
- If the error already came from this package, return it untouched: `return err`. Re-wrapping it would replace the Tag and the stack and break route tests. The errors of gopenvpn-conn are `*errors.Error` too: return them untouched, and branch on the `Tag...` constants of its packages
- Functions return the plain `error` interface, never `*errors.Error` (avoids the non-nil-interface-holding-nil-pointer bug)
- `errors.FromError(err)` is a plain type assertion. It fails if something else wrapped the error, so don't wrap these errors with anything

**Inspecting errors**
- `appErr, ok := errors.FromError(err)`, then use `appErr.Tag`, `appErr.Metadata`, `appErr.Message`
- Use `errors.Is` / `errors.As` / `errors.Unwrap` for the underlying cause stored in `OriginalErr`

**Testing errors**
- Every new failure case gets its own unique Tag and a test that asserts the route, not just that an error occurred:
  ```go
  appErr, ok := errors.FromError(err)
  if !ok {
      t.Fatalf("expected *errors.Error, got %T", err)
  }
  if !appErr.HasRoute("Store.Add->Check.ProfileParseFailed") {
      t.Errorf("unexpected failure path: %s", appErr.Route())
  }
  ```
- A route lists the call chain from outermost to innermost (`Type.Method` or `Func`, joined by `->`) and ends with the Tag. `HasRoute` matches by substring
- Log errors with `slog`, passing the error as an attribute (`l.Error("failed", "err", err)`): `*errors.Error` is a `slog.LogValuer`, so the log gets the Tag, Route, Metadata and stack frames, and the logged object can be replayed in a test via `errors.ErrorJson` and `errors.FromJson`. Do not call `ToJson` or `json.Marshal` on an error yourself

## Security
Tube exists so that an application's traffic goes through the VPN and nowhere else, and it keeps profiles that hold private keys. These rules come before convenience:
- **No leak.** The application's namespace has the loopback and the tunnel, and nothing else: no other interface, no route but into the tunnel, and no way to the machine's own resolver. Never add a path that lets the application reach the network outside the tunnel, not even behind a setting
- **Closed when the VPN is down.** Before the VPN is connected, after it has ended, after the user disconnected, and while another profile is being put in its place, the application has no network at all. Never fall back to the machine's network, and never start the application outside the namespace when something fails: report the error instead
- **The names too.** The application looks names up at the DNS servers of the VPN (or the ones the user set for the profile), through the tunnel. The machine's `resolv.conf` and the socket of its resolver are not visible in the namespace
- **No root.** Never `sudo`, never a setuid helper, never a capability on the binary, and no change to the machine outside the private namespace: no TUN device, address, route, firewall rule or DNS setting of the machine. The application runs as the user who started Tube, with that user's rights
- A change to the namespace, to the routes, to the DNS or to what happens when the VPN ends gets a test of the refusal (the traffic that must not pass does not pass), not only of the success
- The profiles, and every file that holds a key or a password, are written with mode 0600 in folders of mode 0700, atomically. Tests assert the modes
- A key, a password or a token is never logged, never put in an error and never drawn; a password field shows a `*` for each character, never the characters (the user, 2026-11-25: "the password show ******** and not empty"). The text of the clipboard, which the menu of a field reads, may be one of them: it is never drawn or logged either. What a drag of the mouse copies is only what the screen draws there, so a password is copied as its stars; the copied text is never logged
- Everything that comes from a profile or from the VPN server (names, remotes, pushed options, messages of the server) is hostile text: bounded in size, and passed through `sanitize.Display` before it is stored in a name, logged or drawn, so no control character or escape sequence reaches the terminal or a log. What the application prints is hostile text too: the TUI draws only the plain lines that `termlog` made of it
- A profile is only parsed, by `ovpn`: Tube never runs a command that a profile names (`up`, `down`, `script-security`, ...) and never reads a file that a profile names outside the folder it was imported from
- `InsecureSkipVerify` and the like appear only in `_test.go` files

## Git and GitHub
- Never commit or push directly to `main`; work on a branch named `feat/...` or `fix/...`
- **Exception:** if `docs/IMPLEMENTATION.md` exists, follow its Rules section for the steps it lists. Each step is done on its own branch, one at a time. A finished step (all Definition-of-done checks passing, its entry added to `docs/IMPLEMENTED.md` on that branch) is fast-forward merged into the local `main`, without a PR, and tagged `step-<id>` locally. **GitHub gets milestones, not steps:** `main` and the tags are pushed once, when the last step of a milestone (a `## M<n>` section of `docs/IMPLEMENTATION.md`) is merged. Never commit work for a step directly on `main`. Everything not covered by that file (other changes, fixes outside the listed steps) still follows the rule above
- **A finished milestone is merged, tagged and published without asking** (the user, 2026-11-25, asked whether to keep Cave's rule: "Keep it, as in cave"): when the last step of a milestone is on the local `main`, tag that commit with the milestone's version and push `main` and the tags to GitHub, then go on with the next milestone. Do not ask for confirmation or for the version number
- **The version of a milestone** is `v0.<n+1>.0` for milestone `M<n>` (M0 is `v0.1.0`, M1 is `v0.2.0`, ...), tagged with `git tag -a v0.<n+1>.0 -m "M<n>: <title>"`. A publish of `main` between two milestones (a fix outside the steps that the user asked to publish) takes the next patch number of the last version (`v0.2.1` after `v0.2.0`). `v1.0.0` and anything else outside this scheme is the user's decision: ask
- The last step of milestone `M<n>` sets `devVersion` of `internal/app/version.go` to `0.<n+2>.0-dev`, the version the next milestone develops, in its own commit
- Commit messages: imperative mood, e.g. "Add the profile store"
- PRs: short description, link the issue (`Closes #123`), keep them small
- No `.github/` directory and no GitHub Actions workflows: never add them. The Definition-of-done checks are run locally

## Working through the steps
- **Never stop until every step in `docs/IMPLEMENTATION.md` is implemented** (through the last one in the file; as the plan grows, the last step moves). A finished step is never the end of a turn: the turn's last action is a tool call that works on the next step, not a summary. Do not write "starting the next step now" and then end the turn; start it. Report a finished step (and a published milestone) in one or two lines in the middle of the work, and keep working
- **One agent only**: the session works on the steps itself, one at a time; it does not hand work to subagents
- Don't stop to ask for confirmation, and don't end a turn with a question such as "Should I start the next step?", until every step in `docs/IMPLEMENTATION.md` is finished (through the last one). After a step is merged (or a milestone published), report it briefly and start the next step in the same turn
- The only reasons to stop are the ones `docs/IMPLEMENTATION.md` names: an ambiguous step, a wrong spec, a missing tool, a new dependency, `sudo`, a check that keeps failing, a push of a milestone that keeps failing (the push itself needs no question), and a step that says **ask the user**. Then stop, ask, and continue with the same step after the answer
- **Manual checks** don't stop the work (what only a real VPN provider or a real desktop application shows, such as `tube firefox`): add them to `docs/MANUAL-CHECKS.md` (create it if needed) with the step id and the exact command for the user, write "pending, see docs/MANUAL-CHECKS.md" in the step's `IMPLEMENTED.md` entry, and go on. When the user answers, record the answer in that entry and remove the item from the list (delete the file when it is empty)
- A request such as "publish to main" that is already satisfied gets a one-line answer, then work continues with the next step

## Don't
- Don't edit generated files (`*_gen.go`); regenerate them with `go generate ./...`
- Don't commit secrets, `.env` files, real `.ovpn` profiles or keys of real deployments. Keys and profiles in `testdata` are generated for the tests, say so in a `README` next to them, and are used nowhere else
- Don't let a Bubble Tea or Lip Gloss type appear in `internal/core` or `internal/app`
- Don't leave processes running: every Tube instance, `openvpn` server, tmux server or application started for a test, a proof or a check is stopped when it is no longer needed (run it under `timeout -k`, in its own process group), and `pgrep -a` shows nothing left before a task is reported done
- Don't touch the network configuration of the machine: no TUN device, address, route, firewall rule or `tc` rule outside a private namespace, and never `sudo`
- Don't connect to a VPN server on the internet from a test, a proof or a check: they talk to servers they start themselves on the loopback of a private namespace
- Don't start the user's applications from a test, a proof or a check (no `firefox`, no desktop application: the user works on this desktop and sees every stray window). They run small commands of their own choosing (`ping`, a program the test builds)
- Never look for, list, open or read files outside this project folder (no home-directory keys, profiles, configs, logs or other projects). If something outside it is needed, ask the user to provide it. Exceptions: dependency source in the Go module cache (including gopenvpn-conn's), and temporary folders your own tests and scripts create
- Never let a test, proof or check use the user's real Tube profiles, configuration or state: Go tests isolate the folders (`storagetest.Isolate`), and scripts run the program through `scripts/isolated.sh` or `scripts/tui-proof.sh`
- Never let a test, proof or check listen on anything but `127.0.0.1` or `::1` (or an address inside a private namespace), or on a fixed port: ask for port 0 and read the port back

## Memory (OOM)
A run that uses too much memory is killed by the OS (exit 137) and takes the session with it. Be careful:
- Run the race tests with `GOMEMLIMIT=3GiB go test -race -p 2 ./...`. At most two heavy test or build commands run at the same time on this machine, counting those of every agent; the tests against the real server go alone. `scripts/heavy.sh -- <command>` enforces it: it waits for one of two machine-wide slots, and `scripts/heavy.sh --alone -- <command>` takes both. Run every full test run (`./...`) and `golangci-lint run` through it
- Never run Go fuzzing (`-fuzz`) without `-parallel=1`, a short `-fuzztime` (30s at most) and a memory limit (`GOMEMLIMIT=2GiB`, `ulimit -v`); each fuzz worker grows its own memory. Prefer the seed corpus and seeded random tests
- Run the tests of one package (`go test ./internal/core/profiles`) while working; run `./...` only for the final checks
- Tests, proofs and scripts that start `openvpn`, Tube, tmux or an application run under `timeout -k`, and a test that builds big inputs (large profiles, floods of packets) keeps them bounded
- If a command is killed (137), find out why before running it again: it may be a real memory bug (unbounded growth) that needs a fix and a regression test

## Testing notes
- The core and `internal/app` are tested with plain Go tests; no UI needed. What can be tested without a namespace is: the profile store against folders under `t.TempDir()`, the pump of packets against an in-memory tunnel
- Tests of the namespace (the TUN device, its addresses and routes, the DNS files, the application that runs inside) make their own private user and network namespace, with no root, and skip, saying why, on a machine that does not allow it. They run small commands of their own, never the user's applications
- Tests that need the real OpenVPN server carry the `tube_server` build tag and run under `scripts/vpn-ns.sh`: they make the certificates and keys in Go, start `openvpn` on the loopback of the private namespace, and skip when `openvpn` is missing. They never connect to a server on the internet. "The application's traffic goes through the VPN" counts as implemented when such a test shows it, and "nothing leaks" when a test shows the refusal
- TUI tests drive the Bubble Tea model directly (`Update` with key, mouse and window-size messages, then compare `View`) with a fake clock, a fixed color profile and the fake application of `apptest`; golden files are text, without the colors. The add page of the tests lists folders held in memory, never the machine's, and their clipboard is a function of the test, for reading and for copying, never the machine's. No terminal needed
- Proofs of the TUI run it isolated in a private tmux server (`scripts/tui-proof.sh`), never with the user's profiles
- Every error Tag has a route test
