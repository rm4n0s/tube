# Tube: Implementation guide

This file breaks the milestones of `docs/SPEC.md` §18 into small steps. Each step is one unit of work. Work goes strictly top to bottom. Every finished step is recorded in `docs/IMPLEMENTED.md`.

**Who this is for**: coding agents (and humans) implementing Tube. You need the Rules below and the one step you are doing.

---

## Rules

### Before you start a step

1. **Find your step.** It is the first step in this file that has no entry in `docs/IMPLEMENTED.md`. Do only that step. Never work on two steps at once, and never skip ahead.
2. **Read, in this order:**
   1. `CLAUDE.md`, all of it.
   2. The whole step in this file.
   3. Every section of `docs/SPEC.md` listed under **Read**.
   4. `AI_GUIDE.md` of `github.com/rm4n0s/errors` (in the Go module cache), the first time you write error code in a session.
   5. For a step that uses a library (gopenvpn-conn, Bubble Tea, Lip Gloss, `golang.org/x/sys`), the APIs the step names, in the library's source in the Go module cache (`go list -m -f '{{.Dir}}' <module>`). Read the source instead of trusting memory: these libraries change.
3. **Create the branch** from the local `main`:
   ```sh
   git switch main
   git switch -c feat/step-<id>-<slug>     # e.g. feat/step-1.1-profiles
   ```

### While you work

4. **Do exactly the step's Tasks.** Don't do work that belongs to a later step, even if it looks easy. If a later step will need a hook, leave only a comment: `// TODO(step 4.3): ...`.
5. **Follow CLAUDE.md.** In particular: `rm4n0s/errors` for every error, no cgo, no UI import under `internal/core` or `internal/app`, the "Security" section, isolated folders in every test and script, and no server on the internet.
6. **Fail closed.** Where a step leaves a choice between "the application may reach the network some other way" and "the application has no network", it is always the second (SPEC §14).
7. **gopenvpn-conn gaps.** When the library is missing something or has a bug, work around it in Tube, mark the code with `// gopenvpn-conn: <what and why>`, add an entry to `docs/GOPENVPN-CONN-ISSUES.md` (create it if it does not exist), and mention it in the report.
8. **Stop and ask the user** when any of these happens:
   - the step is ambiguous
   - the spec looks wrong, or a decision is needed that it does not make
   - a tool is missing (give the exact install command, e.g. `sudo dnf install openvpn tmux`)
   - a new dependency seems necessary
   - something needs `sudo`
   - a check still fails after a real attempt to fix it
   - the step says **ask the user**

   Never weaken or delete a test, skip a check, or mark a step done just to get unblocked. **Manual checks** don't stop the work: follow CLAUDE.md "Working through the steps".

**Processes you start.** Every Tube instance, keeper, `openvpn` server, tmux server or command you start for a test, a proof or a check is stopped when you are done with it: run it under `timeout -k`, in its own process group, and stop it from `t.Cleanup` or `defer` in Go code. Before you finish a step, check with `pgrep -a openvpn` and `pgrep -a tube` that nothing you started is still running.

**Isolation.** Nothing you run may use the user's real Tube folders or applications, or a server on the internet (SPEC §16.1). Go tests call `storagetest.Isolate(t)` (step 0.3); scripts run the program through `scripts/isolated.sh` (step 0.3) or `scripts/tui-proof.sh` (step 4.2).

### Finishing a step

9. **The standard checks must all pass.** `scripts/checks.sh` (step 0.1) runs them from the repo root, in this order, each heavy one through `scripts/heavy.sh`:
   ```sh
   test -z "$(gofmt -l cmd internal)"
   go vet ./...
   golangci-lint run
   GOMEMLIMIT=3GiB go test -race -p 2 ./...
   CGO_ENABLED=0 go build ./...
   CGO_ENABLED=0 go test ./...
   scripts/error-tags.sh --untested                                # from step 0.2
   scripts/vpn-ns.sh -- go test -race -tags tube_server ./...      # from step 1.4, alone
   ```
   Golden tests have `Golden` in their name; update them with `go test ./internal/tui -run Golden -update-goldens` and look at every changed file.
10. **Proofs.** Only when the step has a **Proof** item:
    - Produce exactly the files it lists, in `docs/implemented-proof-images/`, named `<id>-<name>.txt` (e.g. `4.3-add.txt`), with `scripts/tui-proof.sh` ("Proof methods" below).
    - **Open every proof** and check that it shows what **Proof** describes. If it doesn't, fix the code. The step isn't done.
    - Never write or edit a proof any other way, and never reuse an older one.
11. **Record the step** in `docs/IMPLEMENTED.md` (create it with the first entry), on the step's branch, at the end of the file:
    ```markdown
    ## Step <id>: <title>
    - Date: <YYYY-MM-DD>
    - What was done: <two to six lines>
    - Checks: `scripts/checks.sh` passed (or exactly what was skipped or failed, and why)
    - Proofs: <the files, or "none">
    - Manual checks: <"none", or "pending, see docs/MANUAL-CHECKS.md">
    - Differences from the plan or the spec: <"none", or what and why; the spec is updated on the same branch>
    ```
12. **Merge and tag.** Commit on the branch (`Step <id>: <title>`), then:
    ```sh
    git switch main
    git merge --ff-only feat/step-<id>-<slug>
    git tag -a step-<id> -m "Step <id>: <title>"
    ```
    Never move or delete a tag.
13. **A finished milestone is published** (CLAUDE.md "Git and GitHub"): when the step is the last of its milestone, set `devVersion` to the next milestone's version in its own commit on the step's branch before merging, and after merging:
    ```sh
    git tag -a v0.<n+1>.0 -m "M<n>: <title>"
    git push origin main --tags
    ```
    A push that fails is tried again once; when it still fails, stop and tell the user.
14. **Go on with the next step in the same turn** (CLAUDE.md "Working through the steps").

### Proof methods

**Method U (the TUI).** `scripts/tui-proof.sh <out.txt> [--config <fixture dir>] [--size <cols>x<rows>] [--keys <tmux keys>] [--text <text>] [--wait <regex>] -- [tube arguments]` builds `tube`, runs it isolated in a private tmux server of the given size, sends the keys and the text in the order given, waits for a line that matches, and saves the screen as text. Everything it starts is stopped when it ends. A proof that needs a VPN server runs the script inside `scripts/vpn-ns.sh` with a server that the step's helper starts (step 4.5 says how).

---

## M0: Foundations (v0.1.0)

### Step 0.1: The module and the checks
**Read**: SPEC §3, §4.1, §11 (only `--version`).
**Tasks**:
- `go mod init github.com/rm4n0s/tube`, with `go 1.27.3`.
- `internal/app/version.go`: `devVersion = "0.1.0-dev"` and `Version()`, which returns the value that a release build sets with `-ldflags -X`, else `devVersion`.
- `cmd/tube/main.go`: `tube --version` prints `tube <version>`; anything else prints "not implemented yet" on standard error and exits 125. The logic is in a `run(args, stdout, stderr) int` function that the test calls.
- `.golangci.yml` as Cave's (`go: "1.27"`, `revive` with `exported` and `package-comments`, `testdata` excluded).
- `.gitignore`: add `/tube`, `/dist/`, `*.actual.txt`.
- `scripts/heavy.sh` (two machine-wide slots with `flock`, `--alone` takes both, as Cave's, with lock files named `tube-heavy.*`) and `scripts/checks.sh` (rule 9; a check whose script does not exist yet is skipped with a line that says so).
- `README.md`: the name, one paragraph from SPEC §1, "work in progress", the license.
**Tests**: `run` with `--version`, and with no arguments.
**Done when**: `scripts/checks.sh` passes.

### Step 0.2: The architecture tests and the error-tag tool
**Read**: SPEC §4.2; CLAUDE.md "Error handling".
**Tasks**:
- `go get github.com/rm4n0s/errors@v0.2.0`.
- `internal/archtest`: a test for each of the rules 1 to 6 and 8 to 11 of SPEC §4.2, written over a function that takes the root of a module tree, so that each rule also has a fixture tree under `testdata/` that breaks it and a test that the breach is found. The packages are read with `go/parser` (imports and calls only), for every file of `cmd` and `internal` but `testdata`.
- `internal/archtest/errortags.go` and `internal/archtest/cmd/errortags` (rule 7): list the Tags (the first argument of `errors.New`, `errors.NewErr`, `errors.NewErrf` in non-test files; it must be a string literal or a Tag constant), and with `--untested` the ones that no test file asserts (a route that ends with the Tag, a comparison with `.Tag`, a `wantTag` of a table, or the Tag's constant). `internal/core/errtest.Route` is the assertion that tests use. `docs/ERROR-TAGS-UNTESTED.md` lists exceptions as `- \`Tag\`: reason`; an entry without a reason is an error.
- `scripts/error-tags.sh` runs the tool.
**Tests**: fixtures for every rule and for the tool (a tested Tag, an untested one, a malformed one, an exception).
**Done when**: the checks pass, with `scripts/error-tags.sh --untested` now part of them.

### Step 0.3: Storage, clock, sanitize and logging
**Read**: SPEC §5, §6.5, §15, §16.1.
**Tasks**:
- `internal/core/storage`: `Dirs` (configuration, state) resolved from the environment as §5 says, given as a function of an `env func(string) string` so that tests pass their own; `EnsureDir` (0700, narrows), `WriteFileAtomic` (0600), `NewTempDir` (the temporary folder of a run) and its removal. `storage/storagetest.Isolate(t)`.
- `internal/core/clock`: `Clock` with `Now` and `NewTimer`/`After`; `clock.Real()`; `clock.NewFake(start)` with `Advance`.
- `internal/core/sanitize`: `Display(s string, max int) string` (§6.5), and `HostName(s string) bool` (the check of §7 for search domains).
- `internal/core/logging`: `Open(dir) (*slog.Logger, io.Closer, error)`, JSON lines, mode 0600, the rotation of §15.
- `scripts/isolated.sh` as Cave's, with `TUBE_CONFIG_DIR`, `TUBE_STATE_DIR` and `--config <fixture dir>`.
**Tests**: the folders for each combination of variables; the modes, also with a umask of 0 and a folder that exists as 0755; atomic replacement; `Display` with escape sequences, bidirectional controls, invalid UTF-8, long strings; rotation at the limit; every Tag by its route.
**Done when**: the checks pass. **Last step of M0**: set `devVersion` to `0.2.0-dev`, tag `v0.1.0`, push (rule 13).

---

## M1: Profiles and the VPN (v0.2.0)

### Step 1.1: The store of profiles
**Read**: SPEC §5, §6.1, §6.2 (without the scripts paragraph), §6.4, §6.5. Source: `ovpn.ParseFS`, `ovpn.Profile`, the `Tag...` constants of package `ovpn`.
**Tasks**:
- `go get github.com/rm4n0s/gopenvpn-conn@v1.0.1`.
- `internal/core/profiles`: `Store` over the configuration folder and a `clock.Clock`. `Add(ctx, path, name) (Profile, error)` does §6.2 steps 1 to 4 (the recording file system over `os.Root`, the limits, the copy, the second parse). `List(ctx) ([]Profile, error)` (sorted; broken profiles with their reason; failed adds removed), `Get(ctx, id)`, `Rename`, `SetUsername`, `SetDNS`, `MarkUsed`, `Delete`. `Profile` has the fields of §6.4 and the path of its `files` folder and profile file (for `vpn.Dial`).
- Test profiles are made by a helper of the tests (a CA and certificates generated in Go at test time); nothing secret is committed.
**Tests**: add with inline files and with named files (they are copied, at their relative paths, with the modes of §5); a profile that names a file outside its folder, through `..` and through a symbolic link, is refused; the limits; names (empty, too long, the same without case, hostile); a broken `meta.json`; a failed add is cleaned; rename, user name, DNS (bad address, more than 4), delete; every Tag by its route.
**Done when**: the checks pass.

### Step 1.2: Editing a profile, and profiles that name scripts
**Read**: SPEC §6.2 (the scripts paragraph), §6.3, §14 ("A profile runs nothing").
**Tasks**:
- `Store.Text(ctx, id) ([]byte, error)` and `Store.ReplaceText(ctx, id, text)`: validate as an add does, with the stored files next to it; replace atomically; leave the stored profile as it was when the text is not valid.
- `Store.Add` gets options: `AddOptions{WithoutScripts bool}`. Without it, a profile whose parse fails on a script option returns an error with its own Tag (`ProfileRunsScripts`) and the names of the options in the metadata, so that the caller can ask; with it, the stored copy has those lines turned into comments, as §6.2 says.
**Tests**: a valid edit, an invalid one (the old text stays), an edit that names a new file that is not there; a profile with `up` and `down`: refused with the Tag, added with the option, the comments in the stored copy, the original file unchanged, and nothing was run (the `up` of the test names a program that would make a file).
**Done when**: the checks pass.

### Step 1.3: The tunnel
**Read**: SPEC §7. Source: `ovpn.Profile.Dial`, `openvpn.Conn`, `openvpn.ConnectionState`, `openvpn.Config.Credentials`.
**Tasks**:
- `internal/core/vpn`: `Tunnel`, `Network`, `Credentials{Username, Password}`, `NetworkFromState(openvpn.ConnectionState) (Network, error)` with the checks of §7, and `Dial(ctx, profiles.Profile, *Credentials) (Tunnel, error)`.
- `internal/core/vpn/vpntest`: `NewTunnel(Network)`, a fake `Tunnel` in memory: the test gets the packets that were written and gives the packets to be read, and can make it fail.
**Tests**: `NetworkFromState` (no address, two IPv4 addresses, a bad MTU, search domains with a new line or a space are dropped); `Dial` of a profile that needs credentials with none (`VPNCredentialsNeeded`); `Dial` to a port of `127.0.0.1` where nothing listens returns the library's error untouched, within the context's deadline; the fake tunnel.
**Done when**: the checks pass.

### Step 1.4: The real server in tests
**Read**: SPEC §16.3. gopenvpn-conn's `scripts/vpn-ns.sh` and `internal/servertest` (in the module cache) show one way; write Tube's own.
**Tasks**:
- `scripts/vpn-ns.sh -- <command>`: runs the command in a private user and network namespace with the loopback up, after `go mod download`; sets `TUBE_NS=1`.
- `vpntest` (files with the `tube_server` tag): `StartServer(t, ServerOptions) *Server`: a CA, a server and a client certificate made in Go, a `tls-crypt` key, a server configuration (`dev tun`, `topology subnet`, `server 10.8.0.0 255.255.255.0`, UDP or TCP on `127.0.0.1` and a free port, optional pushed DNS and search domain, optional user name and password checked by a script of the test), `openvpn` started in its own process group and stopped by `t.Cleanup`; `Server.Profile()` gives the path of a client profile; `Server.TunnelAddr()`, `Server.Stop()`, `Server.Start()`. It skips when `openvpn` is missing or `TUBE_NS` is not set.
- `scripts/checks.sh` runs the `tube_server` tests (alone).
**Tests** (tag `tube_server`): `vpn.Dial` of the server's profile, added to a store: the `Network` has the address the server gave and the DNS it pushed; an ICMP echo request written to the tunnel, to the server's tunnel address, gets its reply; a wrong password fails with the library's Tag for it.
**Done when**: the checks pass, the new one included, and `pgrep -a openvpn` shows nothing. **Last step of M1**: `devVersion` `0.3.0-dev`, tag `v0.2.0`, push.

---

## M2: The namespace (v0.3.0)

### Step 2.1: Netlink
**Read**: SPEC §8.3. `golang.org/x/sys/unix`: `NETLINK_ROUTE`, `IfInfomsg`, `IfAddrmsg`, `RtMsg`, `RtAttr`, `NlMsghdr`.
**Tasks**:
- `go get golang.org/x/sys`.
- `internal/core/netlink` (Linux only): `Open() (*Conn, error)`; `LinkIndex(name)`, `LinkUp(index)`, `SetMTU(index, mtu)`, `Addrs(index) ([]netip.Prefix, error)`, `AddAddr`, `DelAddr`, `AddDefaultRoute(index, family)`, `Links() ([]Link, error)` and `Routes() ([]Route, error)` (for the tests of §14). Each request is sent with `NLM_F_ACK` and its answer is read; a kernel error becomes an error with the request's Tag and the `errno`.
- `internal/core/nstest`: `Run(t, name)` runs a function, registered under a name by the test's `TestMain`, in a child of the test binary inside a new user and network namespace (uid mapped to 0), and fails the test with the child's output when it fails; `Require(t)` skips when the machine does not allow it.
**Tests**: inside `nstest`: the loopback is brought up; a TUN made by the test gets an MTU, an IPv4 and an IPv6 address, a default route of each family, and the lists show them; removing an address; errors of the kernel (an index that does not exist) by their route.
**Done when**: the checks pass.

### Step 2.2: The keeper and the TUN device
**Read**: SPEC §8.1, §8.2, §8.8, §3.
**Tasks**:
- `internal/core/sandbox`: the messages of §8.1 and their encoding over a `SOCK_SEQPACKET` socket pair (`SCM_RIGHTS` for the device); `Start(ctx, Options) (*Sandbox, error)` starts the keeper (`/proc/self/exe` by default; the tests give their own binary) and waits for `ready`; `Sandbox.Device() *os.File`; `Sandbox.Close()`. `KeeperMain() int` is the keeper: private mounts, the device, `ready`, then it serves messages (only the end of the socket for now) and exits. `Check(ctx) error` is §8.8, with `SandboxUserNamespaceDenied` and `SandboxTunUnavailable`.
- `cmd/tube/main.go`: when `argv[0]` is `tube-keeper`, call `sandbox.KeeperMain` and exit with its value, before anything else.
**Tests**: the test binary is the keeper when its `argv[0]` says so (`TestMain`). `Start` gives a device; closing the sandbox ends the keeper (no process left); a keeper that never answers (`SandboxKeeperSilent`, with a short deadline); `Check` passes here; the denial is tested by making the start fail (a binary that does not exist, a clone that is refused through an option of the tests).
**Done when**: the checks pass.

### Step 2.3: Addresses, routes and the application
**Read**: SPEC §8.3, §8.5, §8.6, §14.
**Tasks**:
- `Sandbox.Configure(ctx, vpn.Network, dns []netip.Addr) error` and the keeper's side: §8.3, steps 1 to 4 (the DNS files are step 2.4).
- `Sandbox.Run(ctx, argv []string, env []string) error` and `Sandbox.Wait() (ExitStatus, error)`: the keeper starts the application as §8.5 says and reports how it ended. `ExitStatus` has the code or the signal and `Code()`, the number of §10.
- A helper for the tests: `sandboxtest.Peer`, which reads the packets of the device and answers ICMP echo requests (IPv4 and IPv6), and records every packet it saw.
**Tests**: inside the namespace, a probe (the test binary under another name) prints its uid and gid, the interfaces, the addresses and the routes: the user's own uid, `lo` and `tube0` only, one default route for each family that has an address; `ping -c 1` to an address behind the tunnel succeeds when the peer answers, and its request was seen on the device; without a peer it fails; with an IPv4-only network no IPv6 packet is seen; the probe's try to add a route is refused (`EPERM`); the environment has `TUBE=1` and `MOZ_NO_REMOTE=1`; the working folder is the caller's.
**Done when**: the checks pass.

### Step 2.4: The names
**Read**: SPEC §8.4, §14.
**Tasks**:
- The keeper's `configure` writes the DNS files as §8.4 says; a second `configure` writes them again. The content of `resolv.conf` is made by a pure function (`ResolvConf(dns, search) []byte`) that is tested by itself.
- `sandboxtest.Peer` answers DNS queries over UDP for the names a test gives it.
**Tests**: `ResolvConf` (three servers of five, IPv6 servers, no server, search domains); inside the namespace: `/etc/resolv.conf` has the content, the covered folders are empty; `getent hosts <name>` (skipped when `getent` is missing) and a lookup of the probe get the peer's answer, and the query was seen on the device, to the configured server; with no DNS server the lookup fails and no packet leaves; the machine's own `resolv.conf` is unchanged after the test.
**Done when**: the checks pass.

### Step 2.5: Signals and the ways an application ends
**Read**: SPEC §8.1 (the last paragraph), §8.5, §10 ("End", "Signals", "Tube's own failures").
**Tasks**:
- `Sandbox.Signal(sig)`; the keeper passes it on. The keeper ignores SIGINT and SIGQUIT while the application runs. When the socket closes, the keeper ends the application (SIGTERM, then SIGKILL after five seconds; the tests shorten it) and exits.
- `run` of a program that is not found or cannot be executed: `failed` with `SandboxAppNotFound` or `SandboxAppNotExecutable`, which `ExitStatus`/the caller turn into 127 and 126.
**Tests**: an application that exits with 0, 3, and by SIGTERM (`Code()` 143); `Signal` reaches it; an application that ignores SIGTERM is killed after the delay; the socket closing ends the application and the keeper; not found and not executable; no process is left after each test.
**Done when**: the checks pass.

### Step 2.6: The pump
**Read**: SPEC §9.
**Tasks**: `internal/core/pump`: `Start(device io.ReadWriter, tunnel vpn.Tunnel, mtu int) *Pump`, `Pump.Done() <-chan struct{}`, `Pump.Err() error`, `Pump.Stop() Counters`. The device's reads are interrupted by a deadline when it is an `*os.File` that supports it, so that `Stop` returns without closing it.
**Tests**: with a fake device (a pair of channels) and the fake tunnel: packets go both ways unchanged and are counted; a write that the device refuses is counted as dropped and the pump goes on; the tunnel failing ends the pump with that error; `Stop` returns and leaves no goroutine; a second pump on the same device works (reconnect). Inside a namespace: `ping` through sandbox, pump and a fake tunnel that answers.
**Done when**: the checks pass.

### Step 2.7: The session
**Read**: SPEC §10 (without "Reconnecting"), §14, §16.2.
**Tasks**: `internal/core/session`: `Start(ctx, Options) (*Session, error)` with a connected `vpn.Tunnel`, the argv, the profile's name and DNS servers, a logger and a writer for Tube's lines; it makes the temporary folder, starts the sandbox, configures it, warns when there is no DNS server, starts the pump and the application. `Session.Wait() (int, error)` returns the status of §10 and cleans up: the pump, the tunnel, the keeper, the temporary folder. `Session.Signal`. When the tunnel fails, a line is written and the application goes on with no network until it ends (reconnecting is step 5.1).
**Tests**: with the fake tunnel: a run from start to end, the status, everything cleaned (no process, no temporary folder, the tunnel closed); the refusals of §14: a connection made inside before the pump is started fails, after the tunnel failed it fails, and after the process that plays Tube is killed (a helper process of the test) the application inside has no interface but the loopback; the lines and `--quiet`'s writer. With the `tube_server` tag: a session over a real tunnel, `ping -c 2` to the server's tunnel address ends with 0.
**Done when**: the checks pass. **Last step of M2**: `devVersion` `0.4.0-dev`, tag `v0.3.0`, push.

---

## M3: The command line (v0.4.0)

### Step 3.1: Connecting and explaining
**Read**: SPEC §12 (without the events), §12.3, §7.
**Tasks**: `internal/app`: `New(Options) (*App, error)` (the folders, the store, the logger, the clock, the application's argv, the output for Tube's lines); `App.FindProfile(name)` (§11); `App.Connect(ctx, id, *vpn.Credentials) error` (dials, keeps the tunnel, marks the profile used, saves the user name when asked); `App.Run(ctx) (int, error)` (the session); `App.Close()`; `Explain(err) string` with the sentences of §12.3, reading the Tags of gopenvpn-conn from its constants.
**Tests**: `Explain` for each Tag it knows, for an unknown one, and that no sentence has the value of a metadata key called `password` or a line of a profile; `FindProfile` (exact, without case, two that match, none); `Connect` without credentials for a profile that needs them; with the `tube_server` tag: connect and run.
**Done when**: the checks pass.

### Step 3.2: `tube --profile`
**Read**: SPEC §11, §10 ("Tube's own failures", the lines), §8.5 (the last paragraph), §8.8.
**Tasks**: `cmd/tube`: the options of §11 with the `flag` package (both spellings of each), the help text, the look-up of the application in `PATH` (127), the check of §8.8, `--profile` with `--auth-file` or the questions on the terminal (the echo is switched off with termios through `golang.org/x/sys/unix`, and switched on again whatever happens), `--quiet`, the exit statuses, SIGTERM and SIGHUP passed to the session. Without `--profile`: "the TUI comes with milestone M4; use --profile", status 125.
**Tests**: `run` with every option and every mistake of the arguments; the auth file (missing, one line, wider rights than 0600 are refused with a message that says why). With the `tube_server` tag, the real binary, isolated: `tube --profile test ping -c 2 <server>` is 0; `tube --profile test sh -c 'exit 7'` is 7; a name that is resolved by a DNS server of the test on the server's tunnel address; a wrong password is 125 with the sentence of `Explain`; a profile name that does not exist is 125; `tube --profile test nosuchprogram` is 127.
**Done when**: the checks pass. Add to `docs/MANUAL-CHECKS.md`: `tube --profile <a real profile> curl https://ifconfig.me` shows the VPN's address. **Last step of M3**: `devVersion` `0.5.0-dev`, tag `v0.4.0`, push.

---

## M4: The TUI (v0.5.0)

### Step 4.1: Snapshots, commands and events
**Read**: SPEC §12.1, §12.2.
**Tasks**: `internal/app`: `Snapshot`, the commands of §12.2 as methods that return at once and answer with an event, `App.Events() <-chan Event` (or a subscribe function), cancelling a connect. `internal/app/apptest`: `Fake`, which implements the interface that the TUI will use (declare that interface in `internal/app`), with fixed profiles and a script of answers.
**Tests**: each command and its event, with a real store in a temporary folder; add with scripts (the event that asks, then the add without them); a connect that is cancelled; the fake.
**Done when**: the checks pass.

### Step 4.2: The frame, the list and the profiles page
**Read**: SPEC §13.1, §13.2, §13.7, §16.4, §16.5. Source: Bubble Tea v2 (`tea.Model`, `tea.KeyPressMsg`, `tea.WindowSizeMsg`, `tea.View`), Lip Gloss v2. Cave's `internal/tui` is the model to follow (the user, 2026-11-25); when it is not on this machine, follow this file.
**Tasks**:
- `go get charm.land/bubbletea/v2 charm.land/lipgloss/v2`.
- `internal/tui`: `Run(ctx, app, Options) (Result, error)`, the model, the frame (§13.1), the keys, the styles, Tube's own list, the profiles page, the help page, quit. `Result` says whether to run the application.
- The test harness (keys in, `View` out as plain text, a fake clock, a fixed color profile, `-update-goldens`).
- `scripts/tui-proof.sh` (method U).
- `cmd/tube`: without `--profile`, the TUI.
**Tests**: golden files: the empty list, a list with five profiles (one broken, one with a password), the selection moving, a list longer than the page, a small terminal, the help page; hostile names are drawn safe.
**Proof**: `4.2-profiles.txt`: the list with a fixture of three profiles.
**Done when**: the checks pass.

### Step 4.3: Adding and deleting
**Read**: SPEC §13.3, §6.2, §13.2 (`d`).
**Tasks**: Tube's own text field and question; the add page with the completion of paths; the question about scripts; the delete question.
**Tests**: golden files and model tests: typing, moving and deleting in a field; completion (one match, several, none, `~`); a good add returns to the list with the new profile selected; an error of the parser is shown with its option and line; the scripts question (yes and no); delete (yes and no).
**Proof**: `4.3-add.txt` (the add page with an error of the parser), `4.3-delete.txt` (the question).
**Done when**: the checks pass.

### Step 4.4: The profile's page and the editor
**Read**: SPEC §13.4, §13.5, §6.3.
**Tasks**: the page; rename, user name and DNS servers in one field each; the editor through `tea.ExecProcess`, found as §13.5 says (the function that finds it is pure and takes the environment and a look-up function).
**Tests**: golden files of the page (with every kind of content, and long lists cut); each change and its error; the editor found from `VISUAL`, `EDITOR`, `nano`, `vi`, none; a value with arguments; an edit that makes the profile invalid shows the error and keeps the old text (the editor of the test is a program that the test builds).
**Proof**: `4.4-profile.txt`.
**Done when**: the checks pass.

### Step 4.5: Login, connecting and running
**Read**: SPEC §13.6, §10, §12.2.
**Tasks**: the login page (the password field draws nothing), the connecting page with the seconds, cancel, the errors; when connected, the TUI ends and `cmd/tube` runs the session and exits with its status.
**Tests**: golden files of both pages; the password is in no `View`; a wrong password returns to the login page; cancel; the result of `Run`. With the `tube_server` tag: the real binary in a pseudo-terminal that the test makes (`/dev/ptmx` through `x/sys/unix`), keys sent, `ping -c 2` runs and the status is 0.
**Proof**: `4.5-connecting.txt` and `4.5-ran.txt` (the terminal after `tube ping -c 2 <server>`: the output of `ping`), made inside `scripts/vpn-ns.sh` with the server of `vpntest` started by a small program of the tests (`go run -tags tube_server ./internal/core/vpn/vpntest/cmd/server`, which prints the fixture folder and waits).
**Done when**: the checks pass. Add to `docs/MANUAL-CHECKS.md`: `tube firefox` with a real profile (a new Firefox profile, the address shown by a "what is my IP" page, a Firefox that is already open). **Last step of M4**: `devVersion` `0.6.0-dev`, tag `v0.5.0`, push.

---

## M5: Reconnecting (v0.6.0)

### Step 5.1: Connect again
**Read**: SPEC §10 ("Reconnecting"), D14, §14.
**Tasks**: `session` gets a `Dial func(ctx) (vpn.Tunnel, error)` (the App gives one that dials the profile again with the credentials of the run); when the pump ends with an error while the application runs: the line, the delays of §10 on the session's clock, `configure` again, a new pump, the line; a refused login stops the tries; the end of the application or of Tube stops them at once.
**Tests**: with fake tunnels and the fake clock: the delays; a reconnect with other addresses reconfigures the namespace and `ping` works again; between the two tunnels a connection inside fails (the refusal); a refused login; the application ending in the middle of a wait. With the `tube_server` tag: the server is stopped and started again while `ping` runs with a count, and replies come again.
**Done when**: the checks pass. **Last step of M5**: `devVersion` `0.7.0-dev`, tag `v0.6.0`, push.

---

## M6: Release (v0.7.0)

### Step 6.1: The documents and the release build
**Read**: the whole SPEC.
**Tasks**:
- `README.md`: what Tube is, a short example, what it needs (§3), how to build and install, the limits (§17), the license, the note about the OpenVPN trademark.
- `docs/GUIDE.md` for the user: adding a profile, running an application, Firefox (§8.7), DNS servers of a profile, `--profile` in scripts, what the errors mean, the settings of the kernel (§3).
- `docs/SECURITY.md`: the table of §14 in plain words, and what is not promised.
- `scripts/build-release.sh [--version <v>] [--dist <dir>]`: `linux/amd64` and `linux/arm64`, `CGO_ENABLED=0`, `-trimpath`, the version through `-ldflags -X`, a `tar.gz` each with the binary, `LICENSE` and `README.md`, and `SHA256SUMS`. It uploads nothing.
- `docs/MANUAL-CHECKS.md` is complete: every check that only a real VPN provider or a real desktop application shows, with the exact command.
**Tests**: a test with a build tag (`tube_release`) that runs the script into a temporary folder and checks the files, the sums and `tube --version` of the built binary.
**Done when**: the checks pass. **Last step of M6**: `devVersion` `0.8.0-dev`, tag `v0.7.0`, push. Then tell the user what is in `docs/MANUAL-CHECKS.md`.
