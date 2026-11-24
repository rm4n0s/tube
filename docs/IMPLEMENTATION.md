# Tube: Implementation guide

This file breaks the milestones of `docs/SPEC.md` §18 into small steps. Each step is one unit of work for one coding agent session. Work goes strictly top to bottom. Every finished step is recorded in `docs/IMPLEMENTED.md`.

**Who this is for**: coding agents (and humans) implementing Tube. You don't need to read the whole file; you need the Rules, "What is already built", "Traps", the box at the top of your step's milestone, and the one step you are doing.

**How it is written**: steps 0.1 and 0.2 are finished and stay here as a record, in a short form. Every step from 0.3 on is written out in full, so that a model with less experience of this code (Claude Sonnet 5.5 at high effort) can follow it without guessing (the user, 2026-11-25: "write the steps for Sonnet 5.5 high models"). "How the steps are written" below says how to read them.

---

## Rules

### Before you start a step

1. **Find your step.** It is the first step in this file that has no entry in `docs/IMPLEMENTED.md`. Do only that step. Never work on two steps at once, and never skip ahead.
2. **Read, in this order:**
   1. `CLAUDE.md`, all of it.
   2. In this file: "How the steps are written", "What is already built", "Traps", and the box at the top of the step's milestone ("Read this before any step of M2").
   3. The whole step.
   4. Every section of `docs/SPEC.md` listed under **Read**.
   5. `AI_GUIDE.md` of `github.com/rm4n0s/errors` (in the Go module cache: `go list -m -f '{{.Dir}}' github.com/rm4n0s/errors`), the first time you write error code in a session.
   6. For a step that uses a library (gopenvpn-conn, Bubble Tea, Lip Gloss, `golang.org/x/sys`), the APIs the step names, in the library's source in the Go module cache (`go list -m -f '{{.Dir}}' <module>`). Read the source instead of trusting memory, and instead of trusting this file where it says "confirm in the source": these libraries change.
3. **Create the branch** from the local `main` (step 0.3 is the one exception: its branch exists):
   ```sh
   git switch main
   git switch -c feat/step-<id>-<slug>     # e.g. feat/step-1.1-profiles
   ```

### While you work

4. **Do exactly the step's Tasks**, in their order. Don't do work that belongs to a later step, even if it looks easy. If a later step will need a hook, leave only a comment: `// TODO(step 5.3): ...`.
5. **Use the names the step gives**: the files, the types, the functions and the error Tags. Later steps and their tests are written against these names. When a name cannot work (it does not compile, the library has changed), choose the nearest one, and write the difference in the step's `IMPLEMENTED.md` entry.
6. **Follow CLAUDE.md.** In particular: `rm4n0s/errors` for every error, no cgo, no UI import under `internal/core` or `internal/app`, the "Security" section, isolated folders in every test and script, and no server on the internet.
7. **Fail closed.** Where a step leaves a choice between "the application may reach the network some other way" and "the application has no network", it is always the second (SPEC §14).
8. **gopenvpn-conn gaps.** When the library is missing something or has a bug, work around it in Tube, mark the code with `// gopenvpn-conn: <what and why>`, add an entry to `docs/GOPENVPN-CONN-ISSUES.md` (create it if it does not exist), and mention it in the report.
9. **Stop and ask the user** when any of these happens:
   - the step is ambiguous, or two parts of it disagree
   - the spec looks wrong, or a decision is needed that neither the spec nor the step makes
   - a tool is missing (give the exact install command, e.g. `sudo dnf install openvpn tmux`)
   - a new dependency seems necessary
   - something needs `sudo`
   - a check still fails after a real attempt to fix it (three different attempts at most)
   - the step says **ask the user**

   Never weaken or delete a test, skip a check, add a Tag to `docs/ERROR-TAGS-UNTESTED.md` without a reason that is true, or mark a step done just to get unblocked. **Manual checks** don't stop the work: follow CLAUDE.md "Working through the steps".

**Processes you start.** Every Tube instance, keeper, `openvpn` server, tmux server or command you start for a test, a proof or a check is stopped when you are done with it: run it under `timeout -k`, in its own process group, and stop it from `t.Cleanup` or `defer` in Go code. Before you finish a step, check with `pgrep -a openvpn`, `pgrep -a tube` and `pgrep -af '\.test'` that nothing you started is still running.

**Isolation.** Nothing you run may use the user's real Tube folders or applications, or a server on the internet (SPEC §16.1). Go tests call `storagetest.Isolate(t)`; scripts run the program through `scripts/isolated.sh` (step 0.3) or `scripts/tui-proof.sh` (step 5.2).

### Finishing a step

10. **The standard checks must all pass.** `scripts/checks.sh` runs them from the repo root, in this order, each heavy one through `scripts/heavy.sh`:
    ```sh
    test -z "$(gofmt -l cmd internal)"
    go vet ./...
    golangci-lint run
    golangci-lint run --build-tags tube_server
    GOMEMLIMIT=3GiB go test -race -p 2 ./...
    CGO_ENABLED=0 go build ./...
    CGO_ENABLED=0 go test ./...
    scripts/error-tags.sh --untested                                # prints nothing
    scripts/vpn-ns.sh -- go test -race -p 1 -tags tube_server ./...      # from step 1.4, alone
    ```
    While you work, run the tests of the package you are changing (`go test ./internal/core/profiles`), and `scripts/checks.sh --no-server` when you think you are done; run the full `scripts/checks.sh` once at the end. A check that fails is fixed in the code, not in the check.
11. **Proofs.** Only when the step has a **Proof** item:
    - Produce exactly the files it lists, in `docs/implemented-proof-images/`, with the commands it gives ("Proof methods" below).
    - **Open every proof** (the Read tool shows text) and check that it shows what **Proof** describes. If it doesn't, fix the code. The step isn't done.
    - Never write or edit a proof any other way, and never reuse an older one.
12. **Record the step** in `docs/IMPLEMENTED.md`, on the step's branch, at the end of the file:
    ```markdown
    ## Step <id>: <title>
    - Date: <YYYY-MM-DD>
    - What was done: <two to six lines>
    - Checks: `scripts/checks.sh` passed (or exactly what was skipped or failed, and why)
    - Proofs: <the files, or "none">
    - Manual checks: <"none", or "pending, see docs/MANUAL-CHECKS.md">
    - Differences from the plan or the spec: <"none", or what and why; the spec is updated on the same branch>
    ```
13. **Merge and tag.** Commit on the branch (`Step <id>: <title>`, with the attribution line the session gives), then:
    ```sh
    git switch main
    git merge --ff-only feat/step-<id>-<slug>
    git tag -a step-<id> -m "Step <id>: <title>"
    ```
    Never move or delete a tag.
14. **A finished milestone is published** (CLAUDE.md "Git and GitHub"). When the step says **Last step of M<n>**: before merging, set `devVersion` of `internal/app/version.go` to the value the step gives, in its own commit on the step's branch (`Start developing <version>`). After merging and tagging the step:
    ```sh
    git tag -a v0.<n+1>.0 -m "M<n>: <title of the milestone>"
    git push origin main --tags
    ```
    A push that fails is tried again once; when it still fails, stop and tell the user.
15. **Go on with the next step in the same turn** (CLAUDE.md "Working through the steps").

### Proof methods

**Method U (the TUI).** `scripts/tui-proof.sh` (written in step 5.2) builds `tube`, runs it isolated in a private tmux server, sends keys, and saves the screen as text. A proof is looked at, not compared: after making one, open the file and check it against the step's description.

### How the steps are written

Every step from 0.3 on has these parts:

- **Goal**: what exists when the step is done, in one or two sentences.
- **Read**: the sections of `docs/SPEC.md` to read before starting. The spec says *what*; the step says *how*.
- **Files**: every file the step creates or changes. A file that is not listed is not touched, except `go.mod`, `go.sum`, `docs/IMPLEMENTED.md`, and a test file next to a listed file.
- **Tasks**: numbered, done in order. A task names the types and functions with their signatures. Where a task gives a signature, use it. Where it gives a Tag, that Tag is created in exactly that function.
- **Tests**: the tests to write, by name. A test listed here is required; more tests are welcome. "By its route" means with `errtest.Route(t, err, "<route>")`; the route in the step is what is expected, and when the real route differs only in a frame (a closure is `func1`, a helper was needed), use the real one, which `errtest.Route` prints.
- **Proof**: the commands that make the proofs, when the step has any.
- **Finish**: the rules to follow at the end, and what is special (the last step of a milestone).

Words used in the steps: **Tube** is the process the user started; **the keeper** is Tube started again inside the namespace; **the application** is what the user wants to run (`firefox`, `ping`); **the device** is the TUN device `tube0`; **the tunnel** is the VPN connection (`vpn.Tunnel`); **the run screen** is what the TUI shows while the application runs (its tabs: Status, Output, Network).

### What is already built

Read the code of a package before you use it; this list only says what is there.

- **Step 0.1**: the module `github.com/rm4n0s/tube` (Go 1.27.3). `internal/app/version.go`: `devVersion`, `Version()`, and `buildVersion`, which only a release build sets. `cmd/tube/main.go`: `run(args, stdout, stderr) int` with `--version`; everything else prints `tube: not implemented yet` and returns 125. `scripts/heavy.sh` (two machine-wide slots for heavy commands; `--alone` takes both) and `scripts/checks.sh [--no-server]`. `.golangci.yml` (every exported name needs a doc comment, every package a package comment).
- **Step 0.2**: `internal/archtest`: `CheckBoundaries(root)` checks the rules 1 to 6 and 8 to 11 of SPEC §4.2 and `ScanTags(root)` rule 7; both run over the real module in `go test ./internal/archtest`, so a new file that breaks a rule fails that test. `internal/archtest/cmd/errortags` and `scripts/error-tags.sh [--untested [--all]]` list the error Tags. `internal/core/errtest`: `errtest.Route(t, err, parts ...string) map[string]any` fails the test unless `err` is an `*errors.Error` whose route contains every part, and returns the error's metadata. `docs/ERROR-TAGS-UNTESTED.md` is the list of exceptions (empty).
- **Step 0.3, started**: the branch `feat/step-0.3-storage` has a commit "WIP step 0.3" with `internal/core/storage`, `storage/storagetest`, `clock`, `sanitize` and `logging`, taken from Cave (the user's project that Tube's process follows) and partly changed for Tube. It does not build yet. Step 0.3 says what is done and what is left.

This list is not kept up to date by the steps. For a package that a later, finished step made, the step's text in this file and the package's doc comments are the description.

### Traps

These are the mistakes that are easy to make in this project. Read them before every step.

**Errors**

1. **One Tag, one place.** `ScanTags` reports a Tag that is created in two places as a problem, and `scripts/error-tags.sh --untested` then fails. When two functions fail in the same way, they get two Tags, or the failing code moves into one function that both call (and its name is then a frame of the route).
2. **A Tag is a constant whose name starts with `Tag` or `tag`**, declared in the package that creates the error, or a string literal. Exported (`TagProfileNameTaken`) when another package branches on it. `ScanTags` cannot read anything else.
3. **Create the error in the function that fails.** No helper that builds and returns an error. An error inside a closure has the closure in its route, and **the closure's name is not the same in every build**: `Close.func1` without the race detector, `Close.1` with it (seen in step 2.2). So never create an error inside a closure that a test asserts: move the work into a named function or method (`s.closeOnce.Do(func() { s.closeErr = s.close() })`), and assert the route from that function on (`"Sandbox.close.SandboxKeeperExit"`). `errtest.Route` prints the real route when the expected one is not in it.
4. **Never wrap an `*errors.Error`**: the errors of gopenvpn-conn (`ovpn.ParseFS`, `Profile.Dial`, `Conn.Read`) are `*errors.Error` already and are returned untouched. Only an error from the standard library, `x/sys` or Bubble Tea is tagged, with `errors.NewErr`, once.
5. **`io.EOF`, `net.ErrClosed` and `os.ErrDeadlineExceeded` come untouched out of `openvpn.Conn.Read` and `Write`** (the library's contract). They are not `*errors.Error`. Compare them with `errors.Is` of `github.com/rm4n0s/errors`; never import the standard `errors`.
6. **No secret in an error or a log line.** Not a password, not a line of a profile, not the content of a file of a profile. The metadata of a parser's error has the option's name, the line number and a reason, never a value: keep it that way in what you add.

**Tests**

7. `t.Setenv` and `storagetest.Isolate` cannot be used in a test that calls `t.Parallel`.
8. **Inside `scripts/vpn-ns.sh` the tests run as uid 0** (of a user namespace). A test that needs the mode bits to stop it (`notRoot(t)`) skips there, and that is right. Code must use `os.Getuid()` and `os.Getgid()`, never a number and never "0 means root of the machine".
9. **`/proc/self/exe` in a test is the test binary.** A package whose tests start the keeper has a `TestMain` whose first lines are `if sandbox.IsKeeper() { os.Exit(sandbox.KeeperMain()) }` (step 2.2), then `sandboxtest.HandleProbe()` (step 2.3: the test binary is also the application of many tests). Without the first the "keeper" runs the tests again, for ever; without the second the "application" does.
10. **Golden files**: never pass `-update-goldens` to `./...`; name the package (`go test ./internal/tui -run Golden -update-goldens`). Then open every changed file and read it.
11. `gofmt -l cmd internal` also looks at `testdata`: a Go file in `testdata` must be formatted too.
12. **Nothing of the user's.** No test starts `firefox`, reads `~/.config/tube`, or talks to a server on the internet. A test that needs a program uses `ping`, `getent`, `sh` (and skips when it is missing) or the test binary itself under another role.

**The namespace**

13. **Never `signal.Ignore`.** An ignored signal stays ignored in every program that the process starts: `ping` would not stop on Ctrl-C. Tube and the keeper *catch* SIGINT and SIGQUIT with `signal.Notify` and drop them; a caught signal goes back to its default in the program that is started.
14. **Never `Pdeathsig`.** In Go it is tied to the thread that started the child, and a Go thread can end while the program runs. The keeper learns that Tube died from the socket closing (SPEC §8.1).
15. **A file descriptor that must not leak is close-on-exec.** The device's descriptor in Tube, both ends of the socket pair: call `unix.CloseOnExec(fd)` on a descriptor that came over the socket, and pass `SOCK_CLOEXEC` when making a socket. The application must never get the device or the socket.
16. **A bind-mounted file is rewritten in place** (open with `O_TRUNC` and write), never replaced by a rename: the mount shows the old file for ever otherwise.
17. **An IPv6 address is added with `IFA_F_NODAD`**: without it the address is "tentative" for a second and the first connection fails.
18. **The keeper is the only process with rights over the namespace's network.** The application runs in a nested user namespace as the user's own uid. Do not "simplify" this away by running the application as the keeper's uid 0.
19. **No `ip`, no `unshare`, no `mount` command.** Everything is a system call from Go (`golang.org/x/sys/unix`, `syscall`), and `os/exec` is allowed only where rule 5 of SPEC §4.2 says. The scripts of `scripts/` may use them; the program may not.
20. **Stopping is part of the work.** After a test that started a keeper or an application, no process is left and no `tube-*` folder is left in the temporary folder. Tests assert it where the step says so.

**Profiles**

21. **A folder of the store without `meta.json` may be an add that is in progress** in another Tube. Only one that is older than a minute is removed.
22. **`os.Root`** (`os.OpenRoot(dir)`, `root.FS()`) keeps a name from leaving the folder, also through a symbolic link. `os.DirFS` does not. Use `os.Root` wherever a profile names a file.

---

## M0: Foundations (v0.1.0)

### Step 0.1: The module and the checks (finished)

The Go module, `internal/app/version.go`, `cmd/tube` with `--version`, `.golangci.yml`, `scripts/heavy.sh`, `scripts/checks.sh`, a first `README.md`. See `docs/IMPLEMENTED.md`.

### Step 0.2: The architecture tests and the error-tag tool (finished)

`internal/archtest` (the rules of SPEC §4.2, with a fixture tree that breaks each), the `errortags` command, `scripts/error-tags.sh`, `internal/core/errtest`, `docs/ERROR-TAGS-UNTESTED.md`, `github.com/rm4n0s/errors` v0.2.0. See `docs/IMPLEMENTED.md`.

### Step 0.3: Storage, clock, sanitize and logging

**Goal**: the four small packages that everything else stands on, each with its tests, and the script that runs a command away from the user's files.

**Read**: SPEC §5, §6.5, §15, §16.1.

**Files**: `internal/core/storage/storage.go`, `storage_test.go`; `internal/core/storage/storagetest/storagetest.go`, `storagetest_test.go`; `internal/core/clock/clock.go`, `fake.go`, `clock_test.go`; `internal/core/sanitize/sanitize.go`, `sanitize_test.go`; `internal/core/logging/logging.go`, `logging_test.go`; `scripts/isolated.sh`.

**Where the work stands.** This step was started. Do not create a branch: `git switch feat/step-0.3-storage`, then `git rebase main` (the branch was made before this file changed; there is no conflict). The branch has the packages, copied from Cave with the names changed (`Cave` to `Tube`, `CAVE_` to `TUBE_`) and the SPDX line added. `storage.go` and the first half of `storage_test.go` are already rewritten for Tube; the rest is as it was copied. `go vet ./internal/core/...` shows what does not build. Tube is for Linux only and has no instance lock: everything in these files about Windows, about `runtime.GOOS` and about a lock is removed, not kept.

**Tasks**

1. `internal/core/storage/storage.go` is done. Read it; this is its API, which later steps use:
   - `type Dirs struct { Config, State, TempBase string }`, `type Env func(name string) (string, bool)`.
   - `Resolve(env Env) (Dirs, error)`: SPEC §5. Tags `StorageDirNotAbsolute`, `StorageHomeUnknown`.
   - `(Dirs).Ensure(ctx) error` and `EnsureDir(ctx, dir string) error`: mode 0700 whatever the umask, narrowing a wider folder. Tags `StorageEnsureCancelled`, `StorageDirCreateFailed`, `StorageDirNarrowFailed`.
   - `(Dirs).NewTempDir(ctx) (string, error)`: a new folder `tube-*` with mode 0700 under `TempBase`, or under the system's temporary folder when `TempBase` is empty. Tags `StorageTempDirCancelled`, `StorageTempDirFailed`.
   - `WriteFileAtomic(ctx, path string, data []byte) error`: mode 0600, a temporary file in the same folder, `fsync`, rename. Its Tags start with `StorageWrite`.
   - `ReadFile(ctx, path string, maxBytes int64) ([]byte, error)`. Tags `StorageReadCancelled`, `StorageFileNotExist` (matched with `errors.Is(err, storage.ErrNotExist)`), `StorageFileOpenFailed`, `StorageFileReadFailed`, `FileTooLarge`.
   - The constants `DirMode` (0700), `FileMode` (0600), `EnvConfigDir`, `EnvStateDir`.

   Change nothing in it unless a test of task 2 shows a mistake.
2. `storage_test.go`: make it build and pass. `TestResolve`, `TestEnsure`, `TestNewTempDir`, `TestReadFileErrors` and `TestSyncDir` are written for Tube. Between them are the tests that were copied (`TestWriteFileAtomic`, `TestReadFile`, the `failingFS` they use): keep them, and remove from them what Tube does not have: every use of `ErrAlreadyRunning`, `Acquire`, a lock, `Options`, a `Runtime` field, and `runtime.GOOS`. When a whole test is about the lock, delete the test. After this, `scripts/error-tags.sh --untested` must print no Tag of this package: if it prints one, add the missing test for it.
3. `storagetest/storagetest.go` is done: `Isolate(t testing.TB) storage.Dirs` points `HOME`, the XDG variables, `TUBE_CONFIG_DIR` and `TUBE_STATE_DIR` at folders under `t.TempDir()`, creates Tube's two folders, and returns them. Rewrite `storagetest_test.go` for it: one test `TestIsolate` that (a) sets `TUBE_CONFIG_DIR` to a path under the real home (`os.UserHomeDir()`, read before isolating); (b) in a subtest calls `Isolate` and checks that `Config`, `State` and `TempBase` exist and are not inside the real home, that each of `HOME`, `XDG_CONFIG_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`, `XDG_DATA_HOME`, `XDG_RUNTIME_DIR`, `TUBE_CONFIG_DIR`, `TUBE_STATE_DIR` is set and not inside the real home, and that `os.UserHomeDir()` is not the real home; (c) after the subtest checks that the three folders are gone and `TUBE_CONFIG_DIR` has its old value again.
4. `clock`: the copy is complete and has no Windows code. Its API: `type Clock interface { Now() time.Time; NewTicker(d time.Duration) Ticker; AfterFunc(d time.Duration, f func()) Timer }`, `Ticker{C() <-chan time.Time; Stop()}`, `Timer{Stop() bool; Reset(d) bool}`, `clock.System() Clock`, and `clock.NewFake(start time.Time) *Fake` with `Advance(d)` and `Pending() int`. Run its tests; fix only what fails.
5. `sanitize/sanitize.go`: `Display(s string, max int) string` is there: it replaces every unsafe character (invalid UTF-8, control characters, Unicode format characters such as the bidirectional controls and zero-width characters, the line and paragraph separators) with U+FFFD and cuts to `max` characters, the last being `…` when it cut. Change its package comment to say that the text comes from a profile or a VPN server (SPEC §6.5). Add:
   ```go
   // HostName says whether s can be written into resolv.conf as a search domain: 1 to 253
   // bytes, labels of 1 to 63 letters, digits and hyphens (ASCII only) separated by single
   // dots, no label starting or ending with a hyphen. A trailing dot is not accepted.
   func HostName(s string) bool
   ```
6. `logging/logging.go`: keep `Open(ctx context.Context, path string, level slog.Level) (*slog.Logger, io.Closer, error)`, `MaxBytes` and the rotating file. Remove `ParseLevel` and its Tag (Tube has no option for the level). In `openAppend`, after the file is open, call `f.Chmod(0o600)` so that a log that existed with wider rights is narrowed (on failure close the file and return the error, as the `Stat` below it does). In `logging_test.go`: remove `TestParseLevel`, remove the `runtime.GOOS` branches (keep what they guarded when it is about Linux), and name the test files `tube.log`.
7. `scripts/isolated.sh`, executable, in the style of `scripts/checks.sh` (a header comment that is the usage, `set -euo pipefail`):
   ```
   scripts/isolated.sh [--config <fixture dir>] [--keep] -- <command> [args...]
   ```
   It makes a folder with `mktemp -d "${TMPDIR:-/tmp}/tube-iso.XXXXXX"` (mode 700) with the sub-folders `home`, `config`, `state`, `xdg-config`, `xdg-state`, `xdg-cache`, `xdg-data`, `run` (each 700); with `--config`, copies the fixture into `config` (`cp -R "$config"/. "$root/config"/`, then `chmod -R go-rwx`); exports `TUBE_ISOLATED_ROOT`, `TUBE_CONFIG_DIR`, `TUBE_STATE_DIR`, `HOME`, `XDG_CONFIG_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`, `XDG_DATA_HOME`, `XDG_RUNTIME_DIR` to those folders; runs the command; removes the folder in a `trap ... EXIT` (with `--keep` it prints `isolated.sh: kept <folder>` on standard error instead); exits with the command's status, and with 64 for a mistake in its own arguments (a missing `--`, a fixture folder that does not exist).

**Tests**

- `storage`: the tests named in task 2. Check by reading them that these cases exist, and add the ones that do not: a write replaces a file and leaves no temporary file; each failing step of the write (create, write, sync, close, rename, sync of the folder) by its route, with the old file untouched and the temporary file removed; the written file has mode 0600 with a umask of 0; `ReadFile` of a missing file (`errors.Is(err, storage.ErrNotExist)`), of a file one byte longer than the limit, of a cancelled context.
- `sanitize`: the tests of `Display` that were copied, and `TestHostName`, a table: `example.com` (true), `a` (true), `a-b.c-d.example` (true), a name of 253 bytes (true) and of 254 (false), a label of 63 bytes (true) and of 64 (false), the empty string, `.`, `example.com.`, `a..b`, `-a.b`, `a-.b`, `a b`, `a\nb`, `a;b`, `exämple.com`, `a_b.c` (all false).
- `logging`: the copied tests without the level's; add `TestOpenNarrowsAnExistingLog`: a file made with mode 0644 has mode 0600 after `Open`.
- `scripts/isolated.sh`: run it by hand and write what you saw in the `IMPLEMENTED.md` entry: `scripts/isolated.sh -- sh -c 'echo $TUBE_CONFIG_DIR; ls -ld $TUBE_CONFIG_DIR'` prints a folder under the temporary one with mode `drwx------`, and the folder is gone afterwards; `scripts/isolated.sh -- sh -c 'exit 7'; echo $?` prints 7; `scripts/isolated.sh nothing; echo $?` prints 64.

**Finish**: rules 10 to 15. In the commit, squash nothing: the WIP commit stays, and the step's commit comes after it. **Last step of M0**: `devVersion` becomes `0.2.0-dev`; tag `v0.1.0` with the message `M0: Foundations`; push.

---

## M1: Profiles and the VPN (v0.2.0)

**Read this before any step of M1.** These facts about gopenvpn-conn were checked on 2026-11-25 in the source of v1.0.1. Still open the source when you use something that is not listed here.

- Import `openvpn "github.com/rm4n0s/gopenvpn-conn"` (the module's root is package `openvpn`) and `"github.com/rm4n0s/gopenvpn-conn/ovpn"`. Only `internal/core/vpn` (and packages below it) and `internal/core/profiles` may import them, in test files too (SPEC §4.2 rule 3).
- `ovpn.ParseFS(fsys fs.FS, name string) (*ovpn.Profile, error)` opens the profile with `fsys.Open(name)` and each file that the profile names with `fs.ReadFile(fsys, <name relative to the profile's folder>)`. So a file system that wraps `Open` sees every file the profile needs. It refuses an absolute name and one that starts with `..` itself. It reads at most 1 MiB of the profile.
- `ovpn.Profile` has `Config *openvpn.Config`, `Remotes []ovpn.Remote` (`Network` is `"udp"`, `"tcp"`, `"udp4"`, ...; `Address` is `"host:port"`), `NeedsCredentials bool`, `Ignored []string`, and `Dial(ctx) (*openvpn.Conn, error)`.
- The errors of the parser are `*errors.Error` with these Tags (constants of package `ovpn`): `TagUnsupportedOption`, `TagBadOption` (metadata `"option"`, `"line"`, `"reason"`), `TagMissingOption` (`"option"`), `TagFileUnavailable` (`"option"`, `"line"`), `TagBadFile` (`"option"`, `"reason"`), `TagSyntax` (`"line"`), `TagProfileTooLarge`, `TagReadFailed`. The value of `"line"` is an `int`, counted from 1. An option that names a script (`up`, `down`, `route-up`, `route-pre-down`, `up-restart`, `down-pre`, `tls-verify`) is refused with `TagUnsupportedOption`; so is an option the parser does not know (`ipchange`).
- A profile with `auth-user-pass` and no file has `NeedsCredentials` true; set `profile.Config.Credentials = func(ctx context.Context) (openvpn.Credentials, error)` before `Dial`. `openvpn.Credentials` has `Username` and `Password`.
- `*openvpn.Conn` has `Read`, `Write` (one IP packet each), `Close`, `SetReadDeadline`, `SetWriteDeadline`, `SetDeadline` and `ConnectionState() openvpn.ConnectionState`, whose fields Tube uses: `Addrs []netip.Prefix`, `DNSServers []netip.Addr`, `SearchDomains []string`, `MTU int`.
- Tags of package `openvpn` that Tube branches on: `TagAuthFailed` (the server refused the user name or password), `TagServerPollTimeout` and `TagDialFailed` (no server answered), `TagHandshakeTimeout`, `TagLinkFailed`, `TagPingTimeout`, `TagTLSHandshakeFailed`, `TagServerFingerprintMismatch`, `TagServerCertificateUsage`, `TagServerNameMismatch`, `TagServerHalt`, `TagServerRestart`, `TagServerExit`, `TagUnsupportedPushedOption`, `TagCipherNegotiationFailed`, `TagKeyDerivationUnsupported`, `TagPacketTooLarge` (a `Write` larger than the MTU), and `ovpn.TagCredentialsMissing`.

### Step 1.1: The store of profiles

**Goal**: `profiles.Store` adds a profile by copying it and its files into the configuration folder, lists the profiles, and changes and deletes them.

**Read**: SPEC §5, §6.1, §6.2 (not the paragraph about scripts), §6.4, §6.5. Traps 21 and 22.

**Files**: `internal/core/profiles/profiles.go` (the types, the store, list, get, the changes), `add.go` (adding), `load.go` (parsing with the recording file system), `problem.go`, each with its `_test.go`; `internal/core/profiles/profilestest/profilestest.go` (test support).

**Tasks**

1. `go get github.com/rm4n0s/gopenvpn-conn@v1.0.1`, then `go mod tidy`.
2. `load.go`: the one place where a profile is parsed.
   ```go
   // Load parses the profile called file in the folder dir, and returns what the parser made
   // and the names of the files that the profile names, relative to dir, sorted. The folder is
   // opened as an os.Root, so no name leaves it.
   func Load(dir, file string) (*ovpn.Profile, []string, error)
   ```
   - Open the folder with `os.OpenRoot(dir)` (Tag `ProfileFolderOpenFailed`, metadata `"dir"`), close it before returning.
   - Wrap `root.FS()` in a `recordingFS` (a struct with `Open(name string) (fs.File, error)`): it remembers each name that is opened and is not `file`; it returns a file whose `Read` fails once more than `MaxFileBytes` (1 MiB) have been read; and on the 17th different name it fails. In both cases it remembers the *breach* (which limit, which name) in a field.
   - Call `ovpn.ParseFS(recorder, file)`. When it fails and the recorder has a breach, return the breach as Tube's own error, created here in `Load`: `ProfileFileTooLarge` (metadata `"file"`, `"max"`) or `ProfileTooManyFiles` (metadata `"max"`). When it fails without a breach, return the parser's error untouched.
   - The constants `MaxFileBytes = 1 << 20` and `MaxFiles = 16`.
3. `profiles.go`: the types.
   ```go
   // Remote is one server of a profile, as it is shown.
   type Remote struct {
       Network string // "udp" or "tcp", with "4" or "6" when the profile says so
       Address string // "host:port", made safe with sanitize.Display(s, 255)
   }

   // Profile is one stored profile.
   type Profile struct {
       ID       string       // 16 hexadecimal characters; the name of its folder
       Name     string       // what the user sees
       File     string       // the name of the profile file inside FilesDir
       FilesDir string       // <config>/profiles/<id>/files, absolute
       Created  time.Time
       LastUsed time.Time    // the zero time when it was never used
       Username string       // the remembered user name, or ""
       DNS      []netip.Addr // the DNS servers the user set, or none
       Remotes       []Remote
       NeedsPassword bool     // ovpn.Profile.NeedsCredentials
       Ignored       []string // ovpn.Profile.Ignored, each made safe with sanitize.Display(s, 64)
       Files         []string // the files next to the profile file, relative, sorted
       Broken        string   // why the profile cannot be used, or ""; then only ID, Name and FilesDir are set
   }

   type Store struct { /* the profiles folder, a clock.Clock */ }
   func NewStore(configDir string, clk clock.Clock) *Store
   ```
   `meta.json` is a private struct `meta{Version int; Name, File string; Created time.Time; LastUsed *time.Time; Username string; DNS []string}` with the JSON names of SPEC §6.1 (`last_used` is left out when nil). The version is the constant `metaVersion = 1`.
4. The name of a profile, in one function that every caller uses:
   ```go
   // CleanName returns the name as it is stored: trimmed, and made safe with
   // sanitize.Display(name, MaxNameLen).
   func CleanName(name string) (string, error)
   ```
   Trim first; empty is `ProfileNameEmpty`; more than `MaxNameLen` (64) characters (`utf8.RuneCountInString`) is `ProfileNameTooLong` (metadata `"max"`). A second function, private, `(s *Store) nameFree(ctx, name, exceptID string) error` lists the store and returns `ProfileNameTaken` (exported constant `TagProfileNameTaken`, metadata `"name"`) when another profile has the name by `strings.EqualFold`.
5. `add.go`: `func (s *Store) Add(ctx context.Context, path, name string) (Profile, error)`, in this order:
   1. `ctx.Err()` is `ProfileAddCancelled`. `CleanName(name)`; `nameFree`.
   2. `path` is made absolute (`filepath.Abs`, Tag `ProfilePathInvalid`). `dir := filepath.Dir(path)`, `file := filepath.Base(path)`.
   3. `Load(dir, file)`. On an error, return it.
   4. A new id: 8 bytes of `crypto/rand`, as hexadecimal. `storage.EnsureDir` of `<config>/profiles/<id>/files`.
   5. Copy `file` and each file that `Load` returned: read it through a new `os.Root` of `dir` (`root.Open`, then `io.ReadAll(io.LimitReader(f, MaxFileBytes+1))`; Tag `ProfileCopyReadFailed`, metadata `"file"`), `storage.EnsureDir` of the destination's folder when the name has a folder in it, `storage.WriteFileAtomic`.
   6. `Load(filesDir, file)` again, from the store.
   7. Write `meta.json` with `storage.WriteFileAtomic` (`json.MarshalIndent`; Tag `ProfileMetaEncodeFailed` for a marshal that fails, which a test cannot cause: it is the first entry of `docs/ERROR-TAGS-UNTESTED.md`, with the reason "json.Marshal of a struct of strings and times cannot fail").
   8. Return the `Profile`, built by the same private function that `List` uses (task 6).

   When any of 5 to 7 fails, remove `<config>/profiles/<id>` (`os.RemoveAll`; its own failure is ignored, with a comment that says why: the error that matters is the first) and return the error.
6. `func (s *Store) List(ctx context.Context) ([]Profile, error)`:
   - `os.ReadDir` of `<config>/profiles`; a folder that does not exist is an empty list (Tag `ProfileListFailed` for another error).
   - For each entry that is a folder: when it has no `meta.json`, remove it if its modification time is more than a minute before `clock.Now()`, and skip it either way (trap 21).
   - Read `meta.json` with `storage.ReadFile` (64 KiB at most) and `json.Unmarshal`. When that fails, or `Version` is not `metaVersion`, the profile is `Profile{ID: id, Name: id, FilesDir: ..., Broken: "<reason>"}` with the reasons `"its meta.json cannot be read"` and `"it was made by a newer Tube"`.
   - `Load(filesDir, meta.File)`: when it fails, the profile is broken with the reason `"its profile cannot be read: " + sanitize.Display(err.Error(), 200)`, and it keeps its name from the meta.
   - Otherwise fill every field. The DNS strings are parsed with `netip.ParseAddr`; one that does not parse is dropped.
   - Sort by `strings.ToLower(Name)`, then by `ID`.
7. `func (s *Store) Get(ctx context.Context, id string) (Profile, error)`: the id must be 16 hexadecimal characters (Tag `ProfileIDInvalid`), so that it can never be a path; a folder that does not exist is `ProfileNotFound` (exported constant `TagProfileNotFound`, metadata `"id"`). It builds the profile the way `List` does.
8. The changes. Each reads `meta.json`, changes one field and writes it back; they share one private function `(s *Store) update(ctx, id string, change func(*meta) error) error`, which is the one place that creates the Tags `ProfileMetaReadFailed` and `ProfileMetaInvalid` for a meta that cannot be changed.
   - `Rename(ctx, id, name string) error`: `CleanName`, `nameFree(ctx, name, id)`.
   - `SetUsername(ctx, id, username string) error`: `sanitize.Display(strings.TrimSpace(username), 128)`.
   - `SetDNS(ctx, id string, dns []netip.Addr) error`: more than `MaxDNS` (4) is `ProfileTooManyDNS`.
   - `MarkUsed(ctx, id string) error`: `LastUsed` becomes `clock.Now()`.
   - `Delete(ctx, id string) error`: `Get` first (so that a wrong id is an error), then `os.RemoveAll` (Tag `ProfileDeleteFailed`).
   - `ParseDNS(text string) ([]netip.Addr, error)`: splits at spaces and commas, drops empty parts, parses each with `netip.ParseAddr` (Tag `ProfileDNSInvalid`, metadata `"value"` with `sanitize.Display(part, 64)`), refuses the unspecified address (`0.0.0.0`, `::`) with the same Tag, and more than `MaxDNS` with `ProfileDNSTooMany`.
9. `problem.go`: what the rest of Tube needs to know of a parser's error, without importing `ovpn`:
   ```go
   // Problem is what the parser said about a profile.
   type Problem struct {
       Kind   ProblemKind
       Option string // the option's name, or ""
       Line   int    // the line, from 1, or 0
       Reason string // the parser's reason, or ""
   }
   type ProblemKind int
   const (
       ProblemNone        ProblemKind = iota // the error is not the parser's
       ProblemUnsupported                    // ovpn.TagUnsupportedOption
       ProblemBadOption                      // ovpn.TagBadOption
       ProblemMissing                        // ovpn.TagMissingOption
       ProblemFile                           // ovpn.TagFileUnavailable, ovpn.TagBadFile
       ProblemSyntax                         // ovpn.TagSyntax
       ProblemTooLarge                       // ovpn.TagProfileTooLarge
       ProblemUnreadable                     // ovpn.TagReadFailed
   )
   // ProblemOf reads the Tag and the metadata of an error of the parser.
   func ProblemOf(err error) Problem
   ```
   The option and the reason go through `sanitize.Display(s, 64)` and `(s, 200)`; the line is read as an `int`.
10. `profilestest/profilestest.go`, test support (its name ends with `test`, so it may be imported by the tests of any package):
    ```go
    // PKI is a certificate authority with one server and one client certificate, made for a test.
    type PKI struct { CA, ServerCert, ServerKey, ClientCert, ClientKey []byte } // PEM
    func NewPKI(t testing.TB) *PKI
    // Options say what the profile of a test has.
    type Options struct {
        Remote   string   // "127.0.0.1 1194" by default
        Proto    string   // "udp" by default
        Password bool     // adds auth-user-pass
        Separate bool     // ca, cert and key as files next to the profile, not inline
        Extra    []string // more lines
    }
    // WriteProfile writes a client profile into dir and returns its path.
    func WriteProfile(t testing.TB, dir, name string, pki *PKI, opts Options) string
    ```
    The keys are ECDSA P-256 (`ecdsa.GenerateKey`, `x509.CreateCertificate`, `x509.MarshalPKCS8PrivateKey`, PEM types `CERTIFICATE` and `PRIVATE KEY`); the server certificate has `ExtKeyUsageServerAuth` and the DNS name `tube-test-server`, the client's `ExtKeyUsageClientAuth`; all valid from an hour ago for a day. The profile is: `client`, `dev tun`, `proto <proto>`, `remote <remote>`, `nobind`, `remote-cert-tls server`, then the three files inline (`<ca>...</ca>`, `<cert>`, `<key>`) or as `ca ca.crt`, `cert client.crt`, `key client.key`, then `auth-user-pass` when asked, then the extra lines. Nothing of this is ever committed: it is made in `t.TempDir()` each time.

**Tests** (`storagetest.Isolate(t)` gives the configuration folder; a fake clock at a fixed time)

- `TestLoad`: a profile with inline files has no file names; one with separate files has `ca.crt`, `client.crt`, `client.key`, sorted; `ca ../ca.crt` is refused by the parser (`ProblemOf(err).Kind == ProblemFile`, and the file outside was not read: put a marker there that would break the parse if it were); `ca link.crt` where `link.crt` is a symbolic link to a file outside the folder is refused; a named file of `MaxFileBytes+1` bytes is `Load.ProfileFileTooLarge`; a profile that names 17 files is `Load.ProfileTooManyFiles` (use `Extra` with options that take a file; when the parser has fewer than 17 options that take one, reach the limit another way and write how in the test's comment, or lower the limit in the test through an unexported variable); a folder that does not exist is `Load.ProfileFolderOpenFailed`.
- `TestAdd`: inline: the store has `profiles/<id>/meta.json` and `files/<name>.ovpn`; every folder is 0700 and every file 0600 (walk the tree and check each); the returned profile has the name, the remote `127.0.0.1:1194` with `udp`, `NeedsPassword` false. Separate files: the three files are copied. A profile with `auth-user-pass`: `NeedsPassword` true. Deleting the original after the add changes nothing (`Get` still works).
- `TestAddNames`, a table: empty, only spaces, 65 characters (`ProfileNameTooLong`), 64 characters (accepted), a name that exists in another case (`ProfileNameTaken`), a name with an escape sequence (stored with U+FFFD in its place, and `ansi`-free: no byte below 0x20 in `meta.json`).
- `TestAddFailures`: a path that does not exist (the parser's error, untouched: `ProblemOf(err).Kind == ProblemUnreadable` or the Tag of `Load`, whichever the code gives; assert the Tag you see and say in a comment where it comes from); a profile with a syntax error (`ProblemSyntax`, with the line); a cancelled context; after each failure `profiles/` has no folder left.
- `TestList`: empty store; three profiles come sorted without case; a folder without `meta.json` that is two minutes old (set its time with `os.Chtimes` against the fake clock) is removed, one that is ten seconds old stays and is not listed; a `meta.json` that is not JSON gives a broken profile named by its id; a `version` of 2 gives the reason of a newer Tube; a profile file that was damaged after the add gives a broken profile that keeps its name.
- `TestGet`: found; an id of the wrong form (`../x`, 15 characters, capital letters are accepted or not as you decide and document) is `Get.ProfileIDInvalid`; an id that is not there is `ProfileNotFound`.
- `TestChanges`: rename (to a free name, to its own name in another case, to another profile's name); user name (with a control character); DNS (two addresses; five is `ProfileTooManyDNS`); `MarkUsed` sets the fake clock's time; `Delete` removes the folder, and a second `Delete` is `ProfileNotFound`; every change of an id that is not there fails.
- `TestParseDNS`, a table: `""` (none), `"1.1.1.1"`, `"1.1.1.1, 2606:4700::1111"`, `"1.1.1.1 8.8.8.8"`, `"1.1.1.1,,8.8.8.8"`, `"dns.example"`, `"0.0.0.0"`, five addresses.
- `TestProblemOf`: one error of each kind, made by parsing a broken profile, and an error that is not the parser's (`ProblemNone`).
- `TestSecretsStayOut`: add a profile whose `<key>` block holds a marker string (a text that is not a key, so the parse fails): neither `err.Error()` nor any value of the metadata contains the marker.

**Finish**: rules 10 to 15.

### Step 1.2: Editing a profile, and profiles that name scripts

**Goal**: the text of a stored profile can be read and replaced, and a profile that names scripts can be added without them.

**Read**: SPEC §6.2 (the paragraph about scripts), §6.3, §14 ("A profile runs nothing").

**Files**: `internal/core/profiles/text.go`, `scripts.go`, `add.go`, `load.go`, with their tests.

**Tasks**

1. `text.go`:
   - `func (s *Store) Text(ctx context.Context, id string) ([]byte, error)`: `Get`, then `storage.ReadFile` of the profile file (at most 1 MiB).
   - `func (s *Store) ReplaceText(ctx context.Context, id string, text []byte) error`: more than 1 MiB is `ProfileTextTooLarge`. Write the text with `storage.WriteFileAtomic` to `files/.candidate-<8 random hexadecimal characters>.ovpn`; `Load(filesDir, <that name>)`; when it fails, remove the candidate and return the error; when it works, `os.Rename` the candidate over the profile file (Tag `ProfileTextReplaceFailed`). The files next to the profile are not changed: a text that names a file that is not there fails in `Load`.
2. `load.go`: `Load` gets a way to parse a text that is not on the disk. Add a private function `loadWith(dir, file string, override []byte) (*ovpn.Profile, []string, error)` that `Load` calls with `nil`; with a non-nil `override`, the recording file system answers `Open(file)` with a file in memory that holds `override` (a small type with `Read`, `Close` and `Stat`; `Stat` returns an `fs.FileInfo` with the name, the size and mode 0600). Every other name is opened in the folder as before.
3. `scripts.go`:
   ```go
   // scriptOptions are the options that name a program to run. Tube never runs one.
   var scriptOptions = map[string]bool{"up": true, "down": true, "up-restart": true, "down-pre": true,
       "route-up": true, "route-pre-down": true, "ipchange": true, "tls-verify": true}

   // withoutScripts parses the profile called file in dir again and again, each time turning
   // the line of the script option that the parser refused into a comment, and returns the
   // text that parses and the names of the options that were disabled, in the order of
   // their lines.
   func withoutScripts(dir, file string) (text []byte, disabled []string, err error)
   ```
   Read the profile's text (through an `os.Root`, at most 1 MiB). Loop, at most `MaxScriptLines` (16) times: `loadWith(dir, file, text)`; on success return; when `ProblemOf(err)` is `ProblemUnsupported` with an `Option` in `scriptOptions` and a `Line` inside the text, replace that line (lines are split at `"\n"`; the line number counts from 1) with `"# tube: disabled: " + line` and go on; any other error is returned untouched. After 16 lines: `ProfileTooManyScripts`.
4. `add.go`: `Add` gets options:
   ```go
   // AddOptions say how a profile is added.
   type AddOptions struct {
       // WithoutScripts adds a profile that names scripts with those lines disabled.
       WithoutScripts bool
   }
   func (s *Store) Add(ctx context.Context, path, name string, opts AddOptions) (Profile, error)
   ```
   Step 3 of `Add` becomes: `withoutScripts(dir, file)`. When it disabled something and `opts.WithoutScripts` is false, return `errors.New(TagProfileRunsScripts, ...)` (exported constant, value `"ProfileRunsScripts"`, metadata `"options"`: the names joined with `", "`). When it disabled something and the option is set, the stored profile file is the text that `withoutScripts` returned, not the original, and the files are the ones of the parse of that text. The original file is never written. Update the callers and the tests of step 1.1 (`AddOptions{}`).

**Tests**

- `TestText`: the text of an added profile is the file's bytes; of an id that is not there, `ProfileNotFound`.
- `TestReplaceText`: a valid new text (another `remote`) is stored, and `Get` shows the new remote; an invalid one (a syntax error) returns the parser's error, the old text is still there, and `files/` has no `.candidate-` file left; a text that names a file that is not in the store (`ca other.crt`) fails the same way; more than 1 MiB is `ReplaceText.ProfileTextTooLarge`.
- `TestAddWithScripts`: a profile with `script-security 2`, `up /bin/true` and `down "/bin/sh -c 'touch <marker file>'"` (the marker is a path under `t.TempDir()`):
  - without the option: `Add` fails with `TagProfileRunsScripts`, the metadata `"options"` is `"up, down"`, and the store is empty;
  - with it: the profile is added; its stored text has the two lines as `# tube: disabled: up /bin/true` and the other; every other line is unchanged, byte for byte; the original file is unchanged (compare the bytes before and after);
  - in both cases the marker file does not exist afterwards.
- `TestWithoutScriptsLimits`: 17 script lines is `ProfileTooManyScripts`; a profile whose problem is another unsupported option (`comp-lzo`) returns that error untouched, with `Option` `comp-lzo`.
- `TestLoadWithOverride`: the override is parsed, and the files it names are read from the folder.

**Finish**: rules 10 to 15.

### Step 1.3: The tunnel

**Goal**: `vpn.Dial` connects a stored profile and returns a `vpn.Tunnel`; tests have a fake tunnel.

**Read**: SPEC §7.

**Files**: `internal/core/vpn/vpn.go`, `network.go`, `kind.go`, with their tests; `internal/core/vpn/vpntest/tunnel.go` and its test.

**Tasks**

1. `vpn.go`:
   ```go
   // Credentials are the user name and the password of a profile that asks for them.
   type Credentials struct{ Username, Password string }

   // Tunnel is a connected VPN: a stream of IP packets.
   type Tunnel interface {
       Read(packet []byte) (int, error)    // one IP packet from the VPN
       Write(packet []byte) (int, error)   // one IP packet to the VPN
       SetReadDeadline(t time.Time) error  // interrupts a Read, as net.Conn's does
       Network() Network                   // what the server pushed, checked
       Close() error
   }

   // Dial connects the profile and returns when the tunnel is ready.
   func Dial(ctx context.Context, p profiles.Profile, creds *Credentials) (Tunnel, error)
   ```
   `Dial`: a broken profile (`p.Broken != ""`) is `VPNProfileBroken`. `profiles.Load(p.FilesDir, p.File)`. When `NeedsCredentials` and `creds` is nil: `errors.New(TagCredentialsNeeded, ...)` (exported, value `"VPNCredentialsNeeded"`). When `NeedsCredentials`: set `Config.Credentials` to a function that returns them (the profile was just parsed for this call, so nothing else shares its `Config`). `conn, err := profile.Dial(ctx)`: return its error untouched. `NetworkFromState(conn.ConnectionState())`: on an error, close the connection (its error is ignored, with a comment) and return the error. The returned value is a private struct that holds the `*openvpn.Conn` and the `Network`.
2. `network.go`:
   ```go
   // Network is what the namespace needs to know of a tunnel.
   type Network struct {
       Addrs         []netip.Prefix // at most one IPv4 and one IPv6, IPv4 first
       DNSServers    []netip.Addr
       SearchDomains []string
       MTU           int
   }
   func NetworkFromState(state openvpn.ConnectionState) (Network, error)
   ```
   - Addresses: keep the valid ones; unmap an IPv4-mapped address (`Addr().Unmap()`); no address is `VPNNoAddress`; two of the same family is `VPNTooManyAddresses`.
   - MTU: below 576 or above 65535 is `VPNBadMTU` (metadata `"mtu"`).
   - DNS servers: drop the invalid and the unspecified, keep at most 8.
   - Search domains: keep those for which `sanitize.HostName` is true, at most 6.
3. `kind.go`: what the rest of Tube needs to know of a connection's error, without importing gopenvpn-conn:
   ```go
   type Kind int
   const (
       KindOther       Kind = iota
       KindAuthFailed       // openvpn.TagAuthFailed
       KindNoAnswer         // TagServerPollTimeout, TagDialFailed, TagHandshakeTimeout, TagLinkFailed, TagPingTimeout
       KindCertificate      // TagTLSHandshakeFailed, TagServerFingerprintMismatch, TagServerCertificateUsage, TagServerNameMismatch
       KindServerEnded      // TagServerHalt, TagServerRestart, TagServerExit, and io.EOF
       KindUnsupported      // TagUnsupportedPushedOption, TagCipherNegotiationFailed, TagKeyDerivationUnsupported
       KindCancelled        // the context was cancelled or its deadline passed (errors.Is with context.Canceled, context.DeadlineExceeded)
       KindPacketTooLarge   // openvpn.TagPacketTooLarge
   )
   // KindOf says what kind of failure of a connection err is.
   func KindOf(err error) Kind
   ```
   Check `KindCancelled` first: a dial that was cancelled may carry another Tag with the context's error behind it.
4. `vpntest/tunnel.go`:
   ```go
   // Tunnel is a vpn.Tunnel in memory.
   type Tunnel struct { /* ... */ }
   func NewTunnel(network vpn.Network) *Tunnel
   func (t *Tunnel) Deliver(packet []byte)       // the next Read returns a copy of it
   func (t *Tunnel) Written() <-chan []byte      // every packet given to Write, copied; buffered 1024
   func (t *Tunnel) Fail(err error)              // Read and Write return err from now on
   func (t *Tunnel) Closed() bool
   // OnWrite, when set before the tunnel is used, is called with each written packet and may
   // return packets to deliver: a fake server.
   func (t *Tunnel) OnWrite(f func(packet []byte) [][]byte)
   ```
   `Read` waits for a delivered packet, a failure, a close (then `net.ErrClosed`) or the read deadline (then `os.ErrDeadlineExceeded`); a deadline in the past returns at once; setting the zero time removes it. A `Write` of more than `Network().MTU` bytes returns an error with the Tag of `openvpn.TagPacketTooLarge`, as the real one does (`&errors.Error{Tag: openvpn.TagPacketTooLarge}` is enough). It is safe for several goroutines. Add `var _ vpn.Tunnel = (*Tunnel)(nil)`.

**Tests**

- `TestNetworkFromState`, a table: one IPv4; IPv4 and IPv6; an IPv4-mapped IPv6 address becomes IPv4; none; two IPv4; MTU 575, 576, 65535, 65536; DNS with an invalid and an unspecified address and ten valid ones; search domains `example.com`, `bad domain`, `a\nnameserver 6.6.6.6`, eight valid ones.
- `TestDial`: a broken profile (`Dial.VPNProfileBroken`); a profile with `auth-user-pass` and nil credentials (`errtest.Route(t, err, "Dial.VPNCredentialsNeeded")`); a profile whose remote is a UDP port of `127.0.0.1` where nothing listens, with `connect-timeout 1` in the profile and a context of 5 seconds: it fails within 5 seconds, the error is an `*errors.Error`, and `KindOf(err)` is `KindNoAnswer`; the same with a context that is cancelled after 100 ms: `KindCancelled`. (Get the free port with `net.ListenPacket("udp", "127.0.0.1:0")`, read the port, close it.)
- `TestKindOf`, a table of `&errors.Error{Tag: ...}` for every Tag of task 3, plus `io.EOF`, `context.Canceled`, and an error of another kind.
- `vpntest`: deliver then read; read then deliver from another goroutine; `Written` gets a copy (changing the slice after `Write` does not change it); `Fail`; `Close` wakes a waiting `Read`; the deadline; a packet over the MTU; `OnWrite` answering.

**Finish**: rules 10 to 15.

### Step 1.4: The real server in tests

**Goal**: tests with the build tag `tube_server` start a real `openvpn` server in a private namespace and connect to it with `vpn.Dial`.

**Read**: SPEC §16.3. CLAUDE.md "Testing notes" and "Don't".

**Files**: `scripts/vpn-ns.sh`; `internal/core/vpn/vpntest/server.go` (first line after the SPDX line and a blank line: `//go:build tube_server`), `server_test.go` (same tag); `internal/core/vpn/dial_server_test.go` (same tag).

**Facts** (checked on the development machine on 2026-11-25: OpenVPN 2.7 is `/usr/sbin/openvpn`; an ordinary user can run `unshare -Urn`, and inside it is root, has its own loopback, and can make TUN devices):

- A server that the user may run without root needs a namespace where it is root: that is `scripts/vpn-ns.sh`. Inside, `127.0.0.1` is the namespace's own loopback, so a port there is not a port of the machine.
- These server arguments are the ones gopenvpn-conn's own tests use, and they work: `--mode server --tls-server --dev <name> --topology subnet --server 10.8.0.0 255.255.255.0 --proto udp --local 127.0.0.1 --port <port> --ca <file> --cert <file> --key <file> --dh none --verb 4 --log <file> --disable-dco`. The server is ready when its log has the line `Initialization Sequence Completed`. The server's own tunnel address is then `10.8.0.1`, and the first client gets `10.8.0.2`.

**Tasks**

1. `scripts/vpn-ns.sh`, executable:
   ```
   scripts/vpn-ns.sh -- <command> [args...]
   ```
   `cd` to the repo root. Fail with a clear message when `unshare -Urn true` fails (name `user.max_user_namespaces` and, for Ubuntu, `kernel.apparmor_restrict_unprivileged_userns`). Run `go mod download` first (the namespace has no internet). Then `export TUBE_NS=1` and `exec unshare -Urn sh -c 'ip link set lo up && exec "$@"' sh "$@"`. Exit status 2 for a missing `--` or command.
2. `vpntest/server.go`:
   ```go
   // ServerOptions say how the server of a test is set up.
   type ServerOptions struct {
       Proto        string       // "udp" (the default) or "tcp"
       DNS          []netip.Addr // pushed as dhcp-option DNS
       SearchDomain string       // pushed as dhcp-option DOMAIN
       Username     string       // with Password: the server asks for them
       Password     string
       PushIPv6     bool         // also gives the clients an IPv6 address (fd00:8::/64)
   }
   // Server is a running OpenVPN server.
   type Server struct { /* ... */ }
   // StartServer starts openvpn and returns when it is ready. It skips the test when openvpn
   // is not installed or the test does not run under scripts/vpn-ns.sh.
   func StartServer(t testing.TB, opts ServerOptions) *Server
   func (s *Server) ProfilePath() string     // a client profile for this server, in a folder of the test
   func (s *Server) TunnelAddr() netip.Addr  // 10.8.0.1
   func (s *Server) TunnelAddr6() netip.Addr // fd00:8::1, or the zero Addr
   func (s *Server) Stop()                   // ends the process and waits for it
   func (s *Server) Start()                  // starts it again, on the same port, with the same keys
   func (s *Server) Log() string
   ```
   - Skip with `t.Skip` when `exec.LookPath("openvpn")` fails or `os.Getenv("TUBE_NS") != "1"`.
   - The keys come from `profilestest.NewPKI(t)`, written into `t.TempDir()` with mode 0600.
   - The port: listen on `127.0.0.1:0` (UDP or TCP as the protocol), read the port, close. When the server's log says `Address already in use`, take another port, three times at most.
   - The device name is `tt<pid>x<counter>` (15 characters at most), so that two servers of one test do not collide.
   - Besides the arguments of **Facts**: `--keepalive 1 4` (the client then notices a dead server after 4 seconds; step 4.1 needs it); `--push "dhcp-option DNS <addr>"` for each DNS server; `--push "dhcp-option DOMAIN <name>"`; with `PushIPv6`: `--server-ipv6 fd00:8::/64`; with a user name: `--script-security 2 --auth-user-pass-verify <script> via-file --tmp-dir <a folder of the test>`, where the script is a file of the test (mode 0700) that reads the two lines of the file named by `$1` and exits 0 only when they are the user name and the password. Confirm these options in `man openvpn` before you use them.
   - The process runs in its own process group (`SysProcAttr{Setpgid: true}`). Wait for the ready line for at most 10 seconds, reading the log every 25 ms; when the process ends first or the time passes, fail the test with the log.
   - `Stop` sends SIGTERM to the group, waits 3 seconds, then SIGKILL, and waits for the process. `t.Cleanup` calls it. It may be called twice.
   - The client profile is made with `profilestest.WriteProfile` (remote `127.0.0.1 <port>`, the protocol, `Password` when the server asks for one).
3. `scripts/checks.sh` already runs the `tube_server` tests when `scripts/vpn-ns.sh` exists. Run it and see that it does.

**Tests** (all with the tag; each starts its own server)

- `vpntest/server_test.go`: `TestServerStartsAndStops`: after `StartServer` a process `openvpn` runs; after `Stop` it does not (`s.cmd.ProcessState` is set); `Start` brings it back and the log has a second ready line.
- `vpn/dial_server_test.go`:
  - `TestDialRealServer`: add the server's profile to a store (`storagetest.Isolate`, `profiles.NewStore`, `Add`), `vpn.Dial`: the `Network` has the address `10.8.0.2/24`, an MTU of 1500, the DNS server and the search domain that the options pushed. Close the tunnel.
  - `TestPingThroughTheTunnel`: build an IPv4 ICMP echo request by hand (20 bytes of IP header with protocol 1, source the tunnel's address, destination `10.8.0.1`, header checksum; 8 bytes of ICMP with type 8, an id, a sequence number and its checksum; a few bytes of data), `Write` it, and `Read` until an ICMP echo reply with the same id comes (set a read deadline of 5 seconds). Put the two builders (`sandboxtest` will need them in step 2.3) in a new test-support package now: `internal/core/packettest/packettest.go` with `EchoRequest4(src, dst netip.Addr, id, seq uint16) []byte`, `IsEchoReply4(packet []byte, id uint16) bool`, `Checksum(b []byte) uint16`.
  - `TestDialWithPassword`: the right password connects; a wrong one fails with `KindOf(err) == KindAuthFailed`, and neither `err.Error()` nor the metadata has the password in it.
  - `TestDialOverTCP`: `Proto: "tcp"` connects.
  - `TestIPv6`: with `PushIPv6`, the `Network` has two addresses, IPv4 first.

**Finish**: rules 10 to 15. Afterwards `pgrep -a openvpn` shows nothing. **Last step of M1**: `devVersion` becomes `0.3.0-dev`; tag `v0.2.0` with the message `M1: Profiles and the VPN`; push.

---

## M2: The namespace (v0.3.0)

**Read this before any step of M2.** What was shown to work on the development machine on 2026-11-25, with shell commands (`unshare -Urnm`, `ip`, `mount`): an ordinary user can make a user, network and mount namespace; inside it, as uid 0 of the namespace, it can make a TUN device, give it an address, add a default route, write `0 0` to `/proc/sys/net/ipv4/ping_group_range`, and mount a `tmpfs` over `/run/systemd/resolve`; a process started from there in a nested user namespace with the user's own uid mapped back can `ping` (it sends; nobody answered), can read and write the user's home, and gets "Operation not permitted" when it tries to change the device. M2 does the same from Go.

The Go calls below are the ones to use. **Confirm each name and signature in the source** of `golang.org/x/sys/unix` and of the standard library before you use it; when one is different, use what the source has and say so in the step's entry.

- **Starting a process in new namespaces**: `exec.Cmd` with
  ```go
  cmd.SysProcAttr = &syscall.SysProcAttr{
      Cloneflags:                 syscall.CLONE_NEWUSER | syscall.CLONE_NEWNET | syscall.CLONE_NEWNS,
      UidMappings:                []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getuid(), Size: 1}},
      GidMappings:                []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getgid(), Size: 1}},
      GidMappingsEnableSetgroups: false,
  }
  ```
  The whole new process is in the namespaces from its first instruction, so it may mount and configure the network from any goroutine. Never call `unshare` on a running Go program: only one thread would move.
- **The nested user namespace of the application** (started by the keeper, which is uid 0 there): `Cloneflags: syscall.CLONE_NEWUSER`, `UidMappings: {{ContainerID: <the user's uid>, HostID: 0, Size: 1}}`, the same for the gid, `GidMappingsEnableSetgroups: false`. The user's uid and gid are the ones Tube had (`os.Getuid()` in Tube), which Tube tells the keeper.
- **A socket pair**: `unix.Socketpair(unix.AF_UNIX, unix.SOCK_SEQPACKET|unix.SOCK_CLOEXEC, 0)`. One end goes to the child in `cmd.ExtraFiles` (it is file descriptor 3 there); close your copy of that end after `cmd.Start`. Turn an end into a Go connection with `net.FileConn(os.NewFile(uintptr(fd), "name"))`, close the `*os.File`, and assert `*net.UnixConn`: it has deadlines, `WriteMsgUnix(data, oob, nil)` and `ReadMsgUnix(data, oob)`. A `SOCK_SEQPACKET` socket keeps the edges of messages: one write is one read. When the other end is closed, a read returns 0 bytes or `io.EOF`: treat both as "closed".
- **Sending a file descriptor**: `oob := unix.UnixRights(fd)` on the sender; on the receiver `unix.ParseSocketControlMessage(oob[:oobn])`, then `unix.ParseUnixRights(&messages[0])`, then `unix.CloseOnExec(fd)` and `unix.SetNonblock(fd, true)`, then `os.NewFile(uintptr(fd), "tube0")`. A non-blocking descriptor in an `*os.File` has `SetReadDeadline`.
- **The TUN device**: `fd, err := unix.Open("/dev/net/tun", unix.O_RDWR|unix.O_CLOEXEC, 0)`; `ifr, err := unix.NewIfreq("tube0")`; `ifr.SetUint16(unix.IFF_TUN | unix.IFF_NO_PI)`; `unix.IoctlIfreq(fd, unix.TUNSETIFF, ifr)`. With `IFF_NO_PI` each `read` gives one IP packet and each `write` takes one, with nothing in front. The device exists while a descriptor to it is open.
- **Mounts**: `unix.Mount("", "/", "", unix.MS_REC|unix.MS_PRIVATE, "")` makes every mount private (nothing the keeper mounts is seen outside). A `tmpfs`: `unix.Mount("tmpfs", dir, "tmpfs", unix.MS_NOSUID|unix.MS_NODEV|unix.MS_NOEXEC, "size=64k,mode=0755")`. A bind mount of a file over a file: `unix.Mount(source, target, "", unix.MS_BIND, "")`.
- **rtnetlink**: a socket `unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_CLOEXEC, unix.NETLINK_ROUTE)`, bound with `unix.Bind(fd, &unix.SockaddrNetlink{Family: unix.AF_NETLINK})`. A request is a `unix.NlMsghdr`, then a family header (`unix.IfInfomsg`, `unix.IfAddrmsg` or `unix.RtMsg`), then attributes (`unix.RtAttr` with its data), everything in the machine's byte order (`binary.NativeEndian`) and every part padded to 4 bytes. The answers are parsed with the standard library: `syscall.ParseNetlinkMessage(buf[:n])` and `syscall.ParseNetlinkRouteAttr(&message)`. An answer of type `unix.NLMSG_ERROR` holds an `int32` that is 0 (an acknowledgement) or minus an `errno`.
- **Signals** in the keeper and in Tube: `signal.Notify`, never `signal.Ignore` (trap 13).

**The parts of M2 and who calls whom**: `session` (2.7) uses `sandbox` (2.2 to 2.5) and `pump` (2.6); `sandbox` uses `netlink` (2.1). The keeper is the same binary as Tube: `sandbox.KeeperMain()` runs when `argv[0]` is `tube-keeper`.

### Step 2.1: Netlink

**Goal**: a small package that brings a link up, sets its MTU, adds and removes addresses, adds a default route, and lists links, addresses and routes; and the helper that runs a test function inside a new namespace.

**Read**: SPEC §8.3. The box above. `man 7 rtnetlink` and `man 7 netlink` if they are installed.

**Files**: `internal/core/netlink/netlink.go`, `message.go`, `netlink_test.go`, `message_test.go`; `internal/core/nstest/nstest.go`, `nstest_test.go`.

**Tasks**

1. `go get golang.org/x/sys@latest`, then `go mod tidy`.
2. `nstest/nstest.go` (test support):
   ```go
   // Main is the TestMain of a package whose tests run functions inside a namespace. When the
   // process was started by Run, it calls the function and exits; else it runs the tests.
   func Main(m *testing.M, funcs map[string]func() error)
   // Run runs the function called name, in a new process of the test binary inside a new user
   // and network namespace where it is uid 0, with the given variables added to its
   // environment, and returns what it wrote to standard output. It fails the test, with the
   // process's output, when the function returns an error or does not end in 30 seconds.
   func Run(t testing.TB, name string, env ...string) string
   // Require skips the test when this machine does not let the user make such a namespace.
   func Require(t testing.TB)
   ```
   - The name of the function travels in the variable `TUBE_NSTEST_FUNC`. `Main` looks at it first: when it is set, it calls `funcs[name]`, prints an error to standard error and exits with 1, or exits with 0; the name `nstest-probe` is always known and does nothing. When it is not set: `os.Exit(m.Run())`.
   - `Run` calls `Require`, then starts `os.Args[0]` (the test binary) with the clone flags `CLONE_NEWUSER | CLONE_NEWNET` and the mappings of the box above, under a context of 30 seconds, and with `cmd.Cancel` and `WaitDelay` so that a child that hangs is killed.
   - `Require` starts the probe; when the start fails, `t.Skipf("this machine does not allow user namespaces: %v", err)`.
3. `netlink/message.go`: building and reading messages, with no socket, so that it is tested alone:
   - `func request(msgType uint16, flags uint16, seq uint32, header []byte, attrs ...attr) []byte`, where `attr{typ uint16; data []byte}`: the `NlMsghdr` (with `Len` filled in), the family header, each attribute as `RtAttr{Len, Type}` plus data, each padded to 4 bytes.
   - `func ack(messages []syscall.NetlinkMessage, seq uint32) (done bool, errno syscall.Errno)`: reads the `NLMSG_ERROR` message of that sequence number.
4. `netlink/netlink.go` (the file has `//go:build linux` after the SPDX line, as every file of M2 that uses these system calls):
   ```go
   type Conn struct { /* the socket, the next sequence number */ }
   func Open() (*Conn, error)                                   // Tags NetlinkSocketFailed, NetlinkBindFailed
   func (c *Conn) Close() error                                 // Tag NetlinkCloseFailed
   func (c *Conn) LinkIndex(name string) (int, error)           // Tag NetlinkLinkNotFound, metadata "name"
   func (c *Conn) LinkUp(index int) error
   func (c *Conn) SetMTU(index, mtu int) error
   func (c *Conn) AddAddr(index int, prefix netip.Prefix) error
   func (c *Conn) DelAddr(index int, prefix netip.Prefix) error
   func (c *Conn) Addrs(index int) ([]netip.Prefix, error)
   func (c *Conn) AddDefaultRoute(index int, family int) error  // family is unix.AF_INET or unix.AF_INET6
   func (c *Conn) Links() ([]Link, error)
   func (c *Conn) Routes() ([]Route, error)
   type Link struct { Index int; Name string; Up bool; MTU int }
   type Route struct { Family int; Dst netip.Prefix; OutIndex int } // a default route has a Dst of 0 bits
   ```
   - Two private methods do the talking, and they are the only places that create these Tags: `(c *Conn) do(op string, req []byte) error` sends a request with `NLM_F_REQUEST|NLM_F_ACK` and reads until its acknowledgement (`NetlinkSendFailed`, `NetlinkReceiveFailed`, `NetlinkParseFailed`, and `NetlinkRefused` with the metadata `"op"` and `"errno"` (the number) when the kernel answers with an error; make the `errno` reachable with `errors.Is`: `errors.NewErr(TagRefused, errno)`); `(c *Conn) dump(op string, req []byte) ([]syscall.NetlinkMessage, error)` sends with `NLM_F_REQUEST|NLM_F_DUMP` and collects the messages until `NLMSG_DONE` (`NetlinkDumpFailed` for its own failures). The receive buffer is 64 KiB.
   - `LinkUp`: `RTM_NEWLINK` with `IfInfomsg{Family: AF_UNSPEC, Index: index, Flags: IFF_UP, Change: IFF_UP}`.
   - `SetMTU`: `RTM_NEWLINK` with the index and the attribute `IFLA_MTU` (a `uint32`).
   - `AddAddr`: `RTM_NEWADDR` with the flags `NLM_F_CREATE|NLM_F_EXCL`, `IfAddrmsg{Family, Prefixlen, Index}` and the attributes `IFA_LOCAL` and `IFA_ADDRESS`, both the address's bytes (4 or 16). For IPv6 also set `IfAddrmsg.Flags` to `IFA_F_NODAD` (trap 17).
   - `DelAddr`: `RTM_DELADDR` with the same header and attributes.
   - `AddDefaultRoute`: `RTM_NEWROUTE` with `NLM_F_CREATE|NLM_F_EXCL`, `RtMsg{Family: family, Dst_len: 0, Table: RT_TABLE_MAIN, Protocol: RTPROT_STATIC, Scope: RT_SCOPE_LINK for IPv4 and RT_SCOPE_UNIVERSE for IPv6, Type: RTN_UNICAST}` and the attribute `RTA_OIF` (a `uint32`, the index). A route that exists already (`EEXIST`) is not an error: return nil.
   - `Addrs`: a dump of `RTM_GETADDR`; keep the messages whose `IfAddrmsg.Index` is the index; the address is `IFA_LOCAL` when it is there, else `IFA_ADDRESS`.
   - `Links`: a dump of `RTM_GETLINK`; the name is `IFLA_IFNAME` (without its ending zero byte), the MTU `IFLA_MTU`, `Up` is `Flags&IFF_UP != 0`. `LinkIndex` uses it.
   - `Routes`: a dump of `RTM_GETROUTE` for `AF_UNSPEC`; keep the routes of the table `RT_TABLE_MAIN`; `Dst` is `RTA_DST` with `Dst_len` bits, or the zero address of the family with 0 bits; `OutIndex` is `RTA_OIF`.

**Tests**

- `message_test.go`: `request` with no attribute, with one of 4 bytes, with one of 5 bytes (padded to 8): compare the bytes with ones written by hand in the test; `ack` of an acknowledgement, of an error (`EPERM`), of a message with another sequence number.
- `nstest_test.go` (its `TestMain` calls `nstest.Main` with a function `whoami` that prints `os.Getuid()` and the names of `net.Interfaces()`): `TestRun`: the output is `0` and only `lo`; `TestRunPassesTheEnvironment`; `TestRunFailsTheTest` with a recorder (a `testing.TB` that records `Fatalf`, as `internal/core/errtest` has in its tests): a function that returns an error.
- `netlink_test.go`: its `TestMain` calls `nstest.Main` with these functions, and each test is `nstest.Run(t, "<name>")`. The functions run inside the namespace and return an error that says what is wrong:
  - `loopback`: `LinkIndex("lo")`, `LinkUp`, then `Links()` shows `lo` up.
  - `tun`: make a TUN device as the box says (the function does it itself with `unix`; `sandbox` does not exist yet), keep its descriptor open; `SetMTU(1400)`; `AddAddr` `10.9.0.2/24` and `fd00:9::2/64`; `LinkUp`; `AddDefaultRoute` for both families; then `Links()` shows `tube0` with MTU 1400 and up, `Addrs` has both prefixes, `Routes()` has a default route of each family with the device's index; adding the IPv4 default route a second time returns nil.
  - `deladdr`: add and remove an address; `Addrs` no longer has it.
  - `refused`: `LinkUp(9999)` fails; return the error's route and Tag as text on standard output, and let the test, outside, compare it with `Conn.LinkUp->Conn.do.NetlinkRefused` (the function cannot use `errtest.Route`: it has no `testing.T`). To make this easy write one helper in the test file: `routeOf(err error) string`, which returns `appErr.Route()` or `"not an *errors.Error"`, and have every failing case of this file print it and compare outside.
  - `notfound`: `LinkIndex("nosuch0")` is `NetlinkLinkNotFound`.
- Outside the namespace: `TestOpenAndClose` (a socket can be opened by anybody); `TestClosedConn`: after `Close`, `LinkUp(1)` fails with `Conn.do.NetlinkSendFailed`, and a second `Close` fails with `NetlinkCloseFailed`.
- A Tag that only a broken kernel can cause (`NetlinkReceiveFailed`, `NetlinkParseFailed`, `NetlinkBindFailed`, `NetlinkSocketFailed`) is tested when you can find a way that is real (a closed descriptor, a message cut short given to a private function); otherwise it goes to `docs/ERROR-TAGS-UNTESTED.md` with its reason.

**Finish**: rules 10 to 15.

### Step 2.2: The keeper and the TUN device

**Goal**: Tube can start the keeper in its namespace, get the device's file descriptor from it, and end it; and it can check that the machine allows all this.

**Read**: SPEC §8.1, §8.2, §8.8, §3. Traps 9, 14, 15.

**Files**: `internal/core/sandbox/sandbox.go` (Tube's side), `keeper.go` (the keeper), `protocol.go` (the messages), `tun.go` (the device), each with tests; `cmd/tube/main.go`.

**Tasks**

1. `protocol.go`: the messages of SPEC §8.1 as one struct, sent as JSON, one message in each packet of the socket:
   ```go
   type message struct {
       Type     string            `json:"type"`               // "ready", "configure", "configured", "run", "started", "signal", "exited", "failed"
       Network  *networkMessage   `json:"network,omitempty"`  // configure (step 2.3)
       UID, GID int               `json:"uid,omitempty" ...`  // run: the user's own ids
       KillMS   int               `json:"kill_ms,omitempty"`  // run: the delay before SIGKILL
       PID      int               `json:"pid,omitempty"`      // started
       Signal   int               `json:"signal,omitempty"`   // signal; exited by a signal
       Code     int               `json:"code,omitempty"`     // exited
       Tag      string            `json:"tag,omitempty"`      // failed: the keeper's Tag
       Text     string            `json:"text,omitempty"`     // failed: the error's message
       Meta     map[string]string `json:"meta,omitempty"`     // failed: the metadata, as text
       Warnings []string          `json:"warnings,omitempty"` // configured
   }
   func send(conn *net.UnixConn, m message, fd int) error       // fd < 0 sends none
   func receive(conn *net.UnixConn, deadline time.Time) (message, int, error) // the descriptor, or -1
   ```
   A message is at most 32 KiB. `receive` returns an error with the Tag `SandboxSocketClosed` (exported constant) when the other end is closed, `SandboxKeeperSilent` (exported) when the deadline passes (`errors.Is(err, os.ErrDeadlineExceeded)`), `SandboxMessageInvalid` for JSON that does not parse. `send` has `SandboxSendFailed`. A received descriptor is made close-on-exec in `receive`.
2. `tun.go`: `func openTun(path, name string) (*os.File, error)`: the calls of the box. Tags `SandboxTunOpenFailed` (the open) and `SandboxTunSetupFailed` (the `ioctl`), metadata `"path"`. The path is `/dev/net/tun`, or the value of the variable `TUBE_TUN_DEVICE` when it is set (the tests use it to make the open fail).
3. `keeper.go`:
   ```go
   // KeeperArg0 is the argv[0] under which Tube starts itself as the keeper.
   const KeeperArg0 = "tube-keeper"
   // IsKeeper says whether this process was started as the keeper.
   func IsKeeper() bool   // filepath.Base(os.Args[0]) == KeeperArg0
   // KeeperMain is the keeper. It returns the exit status of the process.
   func KeeperMain() int
   ```
   The keeper's arguments: `os.Args[1]` is the absolute path of the application and `os.Args[2:]` its argv (argv[0] included); with no arguments the keeper is only a check (SPEC §8.8). In this step `KeeperMain`:
   1. takes file descriptor 3 as the socket (`net.FileConn`);
   2. makes every mount private (the box);
   3. `openTun`, then sends `ready` with the descriptor, and closes its own copy of the device;
   4. when something of 2 or 3 fails, sends `failed` with the error's Tag, message and metadata (each value as text with `fmt.Sprint`), and returns 1;
   5. reads messages until the socket is closed, answering each one it does not know yet with `failed` and the Tag `KeeperUnknownMessage`; then returns 0.

   The keeper writes nothing to its standard output or standard error: they are the user's terminal. What it has to say goes over the socket.
4. `sandbox.go`:
   ```go
   // Options say how the namespace is started.
   type Options struct {
       Path string            // the application, as an absolute path; "" for a check
       Argv []string          // its argv, argv[0] included
       Env  []string          // the environment of the keeper and the application; nil is os.Environ()
       Dir  string            // the working folder; "" is Tube's
       Stdin, Stdout, Stderr *os.File // nil is Tube's own
       Exe  string            // the binary to start as the keeper; "" is /proc/self/exe
       StepTimeout time.Duration // how long the keeper has to answer; 0 is 10 seconds
       KillDelay   time.Duration // SIGTERM to SIGKILL; 0 is 5 seconds
   }
   type Sandbox struct { /* the command, the connection, the device */ }
   func Start(ctx context.Context, opts Options) (*Sandbox, error)
   func (s *Sandbox) Device() *os.File
   func (s *Sandbox) Close() error
   func Check(ctx context.Context) error
   ```
   - `Start`: the socket pair; the command `exec.Command(exe)` with `cmd.Args = append([]string{KeeperArg0, opts.Path}, opts.Argv...)` (only `{KeeperArg0}` for a check), `cmd.Env`, `cmd.Dir`, the three streams, `cmd.ExtraFiles = []*os.File{<the child's end>}` and the `SysProcAttr` of the box. Start it through a private field `startCmd func(*exec.Cmd) error` (it is `(*exec.Cmd).Start` unless a test sets it). When the start fails: if the cause is `EPERM`, `EACCES`, `ENOSPC`, `EUSERS` or `EINVAL` (`errors.Is`), the error is `SandboxUserNamespaceDenied` (exported constant; metadata `"errno"`), else `SandboxKeeperStartFailed`. Then close the child's end, and `receive` with the step's deadline: `ready` with a descriptor gives the device (`unix.SetNonblock`, `os.NewFile`); `failed` with the keeper's Tag `SandboxTunOpenFailed` or `SandboxTunSetupFailed` becomes `SandboxTunUnavailable` (exported; metadata `"keeper_tag"`, `"text"`); any other `failed` becomes `SandboxKeeperFailed` (metadata `"keeper_tag"`, `"text"`, and the keeper's metadata with the prefix `keeper_`); anything else is `SandboxProtocolError`. On every failure after the start, the keeper is ended as `Close` does.
   - `Close`: closes the socket and the device, waits for the keeper for at most the step's timeout, then kills it and waits. It can be called twice. It returns an error (`SandboxKeeperExit`) only when the keeper ended with a status other than 0 and was not killed by `Close` itself.
   - `Check`: `Start` with no application, then `Close`.
5. `cmd/tube/main.go`: the first lines of `main` become `if sandbox.IsKeeper() { os.Exit(sandbox.KeeperMain()) }`.

**Tests** (`sandbox_test.go` has `TestMain`: `if sandbox.IsKeeper() { os.Exit(sandbox.KeeperMain()) }`, then `nstest.Main(m, funcs)`; every test that starts a keeper calls `nstest.Require(t)` first; `Options.Exe` stays empty, so the keeper is the test binary)

- `TestProtocol`: over a socket pair made in the test: a message goes and comes back equal; a descriptor goes with it and the received one reads the same file; the deadline (`SandboxKeeperSilent`); the other end closed (`SandboxSocketClosed`); bytes that are not JSON, written with `WriteMsgUnix` (`SandboxMessageInvalid`); a message over 32 KiB is refused by `send`.
- `TestStartGivesTheDevice`: `Start` with no application; `Device()` is not nil; a write of a valid packet to it fails or works, but does not panic (the device is down); `Close` returns nil; afterwards the keeper's process is gone (`syscall.Kill(pid, 0)` is `ESRCH`; export the pid to the test with a method in an `export_test.go`).
- `TestTheNamespaceIsNotTheMachines`: after the test, `net.InterfaceByName("tube0")` in the test's own process fails: the device was never in the machine's network.
- `TestKeeperSilent`: `Options.Exe` is a script of the test that sleeps (`#!/bin/sh\nsleep 30`), with a step timeout of 300 ms: `Start.…SandboxKeeperSilent` (use the route you see), and the script's process is gone afterwards.
- `TestKeeperStartFailed`: `Exe` is a path that does not exist: `Start.SandboxKeeperStartFailed`.
- `TestUserNamespaceDenied`: `startCmd` returns `&os.PathError{Op: "fork/exec", Err: syscall.EPERM}`: `Start.SandboxUserNamespaceDenied`.
- `TestTunUnavailable`: `Env` is the environment plus `TUBE_TUN_DEVICE=/nonexistent/tun`: `Start.SandboxTunUnavailable`, and the metadata `keeper_tag` is `SandboxTunOpenFailed`.
- `TestCheck`: passes on this machine.
- `TestIsKeeper`: false in the test process.
- `cmd/tube`: nothing new to test (the keeper's entry is covered here).

**Finish**: rules 10 to 15. `pgrep -af '\.test'` shows nothing left.

### Step 2.3: Addresses, routes and the application

**Goal**: the keeper configures the namespace's network, starts the application as the user, and reports how it ended. A test plays the VPN on the device.

**Read**: SPEC §8.3, §8.5, §8.6, §14. Traps 13, 17, 18.

**Files**: `internal/core/sandbox/sandbox.go`, `keeper.go`, `protocol.go`, `configure.go` (the keeper's configuring), `status.go`; `internal/core/sandbox/sandboxtest/peer.go`, `probe.go`; `internal/core/packettest/packettest.go`; tests.

**Tasks**

1. `protocol.go`: `networkMessage{Addrs []string; MTU int; DNS []string; Search []string}` (prefixes and addresses as text; the keeper parses them again with `netip.ParsePrefix` and `netip.ParseAddr` and refuses what does not parse with the Tag `KeeperNetworkInvalid`).
2. `configure.go`: `func configureNetwork(nl *netlink.Conn, n networkMessage) error`, in the keeper, in this order (SPEC §8.3):
   1. `lo`: `LinkIndex`, `LinkUp`.
   2. Whether there is an IPv6 address decides IPv6 on the device: write `0` or `1` to `/proc/sys/net/ipv6/conf/tube0/disable_ipv6` (`1` when there is none). A file that does not exist (a kernel without IPv6) is not an error. Tag `KeeperSysctlFailed`, metadata `"file"`, for another failure. Do this before adding addresses.
   3. `tube0`: `SetMTU`; `Addrs`, and `DelAddr` of each one that is not in the new list (leave link-local addresses, `fe80::/10`, alone); `AddAddr` of each new one that is not there; `LinkUp`.
   4. `AddDefaultRoute` for each family that has an address.
   5. Write `0 0` to `/proc/sys/net/ipv4/ping_group_range` (the same Tag as 2).

   The `configure` message is answered with `configured`, or with `failed` and the error. The keeper opens one `netlink.Conn` and keeps it.
3. `status.go`:
   ```go
   // ExitStatus is how the application ended.
   type ExitStatus struct {
       Code   int            // the exit code, when Signal is 0
       Signal syscall.Signal // the signal that ended it, or 0
   }
   // Status is the exit status that Tube ends with: the code, or 128 plus the signal.
   func (s ExitStatus) Status() int
   ```
4. `keeper.go`: the `run` message (it has the user's uid and gid and the kill delay). The keeper starts `os.Args[1]` with the argv `os.Args[2:]`, its own environment and working folder, its own three streams, and the nested user namespace of the box. When the start fails: `failed` with the Tag `KeeperAppNotFound` (the cause is `ENOENT`) or `KeeperAppNotExecutable` (`EACCES`, `ENOEXEC`, a folder) or `KeeperAppStartFailed`. When it started: `started` with the pid; then one goroutine waits for the application and, when it ended, the keeper sends `exited` (the code, or the signal: `exec.ExitError.Sys().(syscall.WaitStatus)`, `Signaled()`, `Signal()`) and returns 0. The loop that reads messages goes on while the application runs (a second `configure` must work: step 4.1). A second `run` is `failed` with `KeeperAlreadyRunning`.
5. `sandbox.go`:
   ```go
   func (s *Sandbox) Configure(ctx context.Context, n vpn.Network, dns []netip.Addr) error
   func (s *Sandbox) Run(ctx context.Context) error
   func (s *Sandbox) Wait() (ExitStatus, error)
   ```
   - One goroutine, started by `Start`, reads every message of the keeper and hands it to whoever waits: a channel for the answers to requests, and a channel that is closed with the `exited` message (or with the error when the socket closes first). So `Wait` and a later `Configure` do not fight over the socket. Only one request is in flight at a time (a mutex).
   - `Configure` sends `configure` (the DNS servers and search domains are sent now and used in step 2.4) and waits for `configured` (the step's timeout). `failed` becomes `SandboxConfigureFailed` (metadata as in `Start`).
   - `Run` sends `run` with `os.Getuid()`, `os.Getgid()` and the kill delay, and waits for `started`. `failed` with `KeeperAppNotFound` becomes `SandboxAppNotFound`, with `KeeperAppNotExecutable` becomes `SandboxAppNotExecutable` (both exported constants), any other `SandboxRunFailed`.
   - `Wait` blocks until `exited` and returns the status. When the socket closes before `exited`: `SandboxKeeperLost`.
   - The environment: `Start` adds nothing itself; the caller (`session`, step 2.7) puts `TUBE=1`, `TUBE_PROFILE` and `MOZ_NO_REMOTE=1` into `Options.Env`. For this step the tests put them there.
6. `packettest`: add what the peer needs: `EchoReply4(request []byte) []byte` (swap the addresses, type 0, new ICMP checksum), `IsEchoRequest4(packet []byte) bool`, the same three for IPv6 (`EchoRequest6`, `IsEchoRequest6`, `EchoReply6`; ICMPv6 type 128 and 129; its checksum covers a pseudo-header of the two addresses, the length and the next header 58), `Version(packet []byte) int` (4, 6 or 0), `Src(packet)`, `Dst(packet) netip.Addr`.
7. `sandboxtest/peer.go`:
   ```go
   // Peer plays the VPN for a test: it reads the packets that leave the namespace through the
   // device and answers some of them.
   type Peer struct { /* ... */ }
   // NewPeer starts reading the device. It stops when the test ends.
   func NewPeer(t testing.TB, device *os.File) *Peer
   func (p *Peer) AnswerPings(on bool)        // off by default
   func (p *Peer) Packets() [][]byte           // a copy of every packet seen so far
   func (p *Peer) WaitFor(t testing.TB, d time.Duration, match func(packet []byte) bool) []byte // fails the test when none comes
   ```
   The reader stops through a read deadline that `t.Cleanup` sets; a read error after that is expected.
8. The probes: the application of most tests of M2 and later is the test binary itself, in a role that a variable names. They live in one place, so that the tests of `sandbox`, `pump`, `session` and `app` share them: `sandboxtest/probe.go` has
   ```go
   // HandleProbe runs the probe that the variable TUBE_SANDBOX_PROBE names, and exits; it
   // returns when the variable is not set. A TestMain calls it right after the keeper check.
   func HandleProbe()
   // ProbeEnv is the variable that makes the test binary run the probe: "TUBE_SANDBOX_PROBE=<name>".
   func ProbeEnv(name string) string
   ```
   and one function for each probe. `sandboxtest` must not import `sandbox` (the tests of `sandbox` import it); a probe uses `netlink`, `net` and the standard library. Later steps add their probes here (`lookup`, `files`, `stubborn`, `sigint`, `ping-twice`, `linger`, `ping-on-demand`); the arguments of a probe are the application's arguments. The probe `report` prints one JSON object: `uid`, `gid`, `cwd`, `env` (the values of `TUBE`, `TUBE_PROFILE`, `MOZ_NO_REMOTE`), `links` (name, up, MTU, from `netlink.Links`), `addrs` (of `tube0`), `routes` (from `netlink.Routes`), and `add_route_error` (the text of the `errno` that `AddDefaultRoute` returns when the probe tries it, or `""`). The tests give the sandbox a temporary file as `Stdout` and read it after `Wait`.

**Tests** (each calls `nstest.Require(t)`; a helper `startSandbox(t, argv, env)` does `Start`, registers `Close` with `t.Cleanup`, and returns the sandbox and a peer)

- `TestConfigureAndReport`: configure with `10.9.0.2/24` and MTU 1400; run the probe `report`; `Wait` is status 0. The report: `uid` and `gid` are the test's own `os.Getuid()` and `os.Getgid()`; `links` are exactly `lo` and `tube0`, both up, `tube0` with MTU 1400; `addrs` is `10.9.0.2/24`; `routes` has one default route, of IPv4, out of `tube0`; `add_route_error` is not empty (`operation not permitted`); `env` has `TUBE=1` and `MOZ_NO_REMOTE=1`; `cwd` is the test's.
- `TestConfigureIPv6`: with `10.9.0.2/24` and `fd00:9::2/64`: two default routes; without the IPv6 address: the peer sees no IPv6 packet at all during the test (`Version(p) == 6` for none of `peer.Packets()`).
- `TestConfigureTwice`: configure, then configure with `10.10.0.5/24`: the report shows only the new address and still one default route.
- `TestPing` (skips when `ping` is not in `PATH`): the application is `ping -c 1 -W 2 10.9.0.1`; with `AnswerPings(true)` the status is 0 and the peer saw an echo request from `10.9.0.2` to `10.9.0.1`; with it off, the status is not 0.
- `TestPing6` (skips without `ping`): the same to `fd00:9::1`.
- `TestExitCodes`: the application `sh -c 'exit 3'` gives `ExitStatus{Code: 3}`, `Status()` 3; `sh -c 'kill -TERM $$'` gives `Signal: SIGTERM`, `Status()` 143.
- `TestStatus`, a table of `ExitStatus` values and their `Status()`.
- `TestRunTwice`: a second `Run` fails with `SandboxRunFailed` and the keeper's Tag `KeeperAlreadyRunning`.
- `TestConfigureRefusesBadNetworks`: send a `networkMessage` with an address that does not parse (through an unexported method of the test): `SandboxConfigureFailed` with the keeper's Tag `KeeperNetworkInvalid`.
- `packettest`: the checksum of a known packet; a request turned into a reply is recognized; the same for IPv6.

**Finish**: rules 10 to 15.

### Step 2.4: The names

**Goal**: inside the namespace, names are looked up at the DNS servers of the run, through the device, and the machine's resolver cannot be reached.

**Read**: SPEC §8.4, §14. Trap 16.

**Files**: `internal/core/sandbox/dns.go` (the keeper's DNS files, and `ResolvConf`), `keeper.go`, `sandbox.go`; `internal/core/dnstest/dnstest.go`; `internal/core/sandbox/sandboxtest/peer.go`; tests.

**Facts** (checked on the development machine on 2026-11-25): `/etc/resolv.conf` is a symbolic link to `../run/systemd/resolve/stub-resolv.conf`; `/etc/nsswitch.conf` has `hosts: files myhostname mdns4_minimal [NOTFOUND=return] resolve [!UNAVAIL=return] dns`; `/run/systemd/resolve` and `/run/avahi-daemon` exist. Other machines have a plain file `/etc/resolv.conf`. The code must work for both.

**Tasks**

1. `dns.go`, the pure part:
   ```go
   // ResolvConf is the content of resolv.conf for a run: a comment line that says Tube wrote
   // it, a nameserver line for each of the first 3 servers, a search line when there are
   // search domains (the first 6), and "options edns0". With no server it has a comment that
   // says that the VPN gave none, and no nameserver line.
   func ResolvConf(dns []netip.Addr, search []string) []byte
   ```
   An IPv6 server is written as it is (`nameserver fd00::53`); an address with a zone is dropped. A search domain for which `sanitize.HostName` is false is dropped here too: this function is the last guard before the file.
2. `dns.go`, the keeper's part: `type dnsFiles struct{...}` with `func (d *dnsFiles) apply(dns []netip.Addr, search []string) (warnings []string, err error)`, called on every `configure`.
   - The first time only: find where `/etc/resolv.conf` leads, **before** covering anything: follow the symbolic links by hand, at most 8 times (`os.Lstat`, `os.Readlink`, a relative target is joined to the link's folder, `filepath.Clean`), so that the answer is a path even when the last target does not exist. Call it the *target*.
   - The first time only: cover each of `/run/systemd/resolve`, `/run/nscd`, `/var/run/nscd`, `/run/avahi-daemon` that exists with an empty `tmpfs` (the box). Resolve each with `filepath.EvalSymlinks` first and cover a folder once (`/var/run` is a link to `/run` on most machines). Tag `KeeperCoverFailed`, metadata `"dir"`.
   - Then, every time, write the content of `ResolvConf`:
     - when the target is inside a covered folder: make its folder (`os.MkdirAll`, 0755) and write the file (0644) in the `tmpfs`;
     - else when the target exists: the first time, write the content to the file `resolv.conf` of the keeper's temporary folder (the variable `TUBE_KEEPER_TMP`, which `Start` sets from `Options.TempDir`; add that field) with mode 0644 and bind-mount it over the target (Tag `KeeperResolvMountFailed`); later times, rewrite that file in place (trap 16);
     - else (no target, not covered): no file can be given; add the warning `"this machine has no resolv.conf: names will not be found"` and go on. The C library then asks `127.0.0.1`, where nothing listens.
   - With no DNS server at all, add the warning `"the VPN gave no DNS server: names will not be found; set DNS servers for the profile"`.
   - Tag `KeeperResolvWriteFailed` for a write that fails.
3. `keeper.go`: `configure` calls `apply` after the network is configured, and puts the warnings into `configured`. `sandbox.go`: `Configure` returns them: its signature becomes `Configure(ctx, n vpn.Network, dns []netip.Addr) (warnings []string, err error)`. The servers that are sent are `dns` when it is not empty, else `n.DNSServers`; the search domains are `n.SearchDomains`.
4. `dnstest/dnstest.go` (test support; no dependency: the DNS messages are built by hand):
   ```go
   // Question reads the first question of a DNS query: the name (lower case, no ending dot),
   // the type (1 is A, 28 is AAAA) and the query's id.
   func Question(query []byte) (name string, qtype uint16, id uint16, ok bool)
   // Answer builds the answer to a query: one A record with the address when the query asks
   // for A and addr is IPv4 (one AAAA record for AAAA and IPv6), an empty answer (no error,
   // no record) for another type or an invalid addr, and NXDOMAIN when found is false.
   func Answer(query []byte, addr netip.Addr, found bool) []byte
   // Query builds a query for the name and the type.
   func Query(id uint16, name string, qtype uint16) []byte
   ```
5. `sandboxtest/peer.go`: `func (p *Peer) AnswerDNS(names map[string]netip.Addr)`: a UDP packet over IPv4 to port 53 whose payload is a query gets an answer (the IP and UDP headers with the addresses and ports swapped; a UDP checksum of 0 is allowed over IPv4): the address for a known name, NXDOMAIN for another. `packettest` gets `UDP4(src, dst netip.AddrPort, payload []byte) []byte` and `ParseUDP4(packet) (src, dst netip.AddrPort, payload []byte, ok bool)`.
6. The probes: `lookup <name>` resolves the name with `(&net.Resolver{PreferGo: true}).LookupHost` under a context of 5 seconds and prints the addresses or the error; `files` prints the content of `/etc/resolv.conf` and, for each folder of the cover list that exists, the names in it.

**Tests**

- `TestResolvConf`, a table compared with whole expected texts: one server; five servers (three lines); an IPv6 server; a server with a zone (dropped); no server (the comment, no `nameserver`); two search domains; eight (six kept); a search domain with a new line in it (dropped).
- `dnstest`: `Query` then `Question` gives the name, the type and the id back; `Answer` for A, for AAAA with an IPv4 address (empty answer), for NXDOMAIN; a query cut short is `ok == false`.
- Inside the namespace (`nstest.Require`):
  - `TestResolvConfInside`: configure with the DNS server `10.9.0.1` and the search domain `tube.test`; the probe `files` shows a `resolv.conf` with `nameserver 10.9.0.1` and `search tube.test` and nothing of the machine's servers, and every covered folder that exists holds at most what Tube put there.
  - `TestLookupGoesThroughTheDevice`: the peer answers `tube-test.example` with `192.0.2.7`; the probe `lookup tube-test.example` prints `192.0.2.7`; the peer saw a UDP packet to `10.9.0.1:53` whose question is that name.
  - `TestGetent` (skips when `getent` is not in `PATH`): the application `getent hosts tube-test.example` ends with status 0, and its output (the sandbox's `Stdout` file) has `192.0.2.7`. This is the C library's path, the one applications use.
  - `TestTheProfilesDNSWins`: `Configure` with `dns` `10.9.0.9` and a network whose `DNSServers` is `10.9.0.1`: the query goes to `10.9.0.9`.
  - `TestNoDNSServer`: configure with none: `Configure` returns the warning; the probe's lookup fails; the peer saw no packet to port 53 on any address but `127.0.0.1` (nothing to `127.0.0.1` can reach the device at all: assert that the peer saw no UDP packet).
  - `TestConfigureTwiceRewritesTheFile`: a second `Configure` with another server changes what `files` shows (run the probe after the second configure).
  - `TestTheMachinesResolvConfIsUntouched`: read the test process's own `/etc/resolv.conf` before and after a sandbox ran: the same bytes. (The test reads a file of the machine, not of the user; that is allowed here because the promise is about that file.)

**Finish**: rules 10 to 15.

### Step 2.5: Signals and the ways an application ends

**Goal**: a signal reaches the application; the application is ended when Tube dies; "not found" and "cannot be executed" are told apart; and the application can be given a terminal of its own (the TUI will show its output, SPEC §8.5).

**Read**: SPEC §8.1 (the last paragraph), §8.5, §10 ("End", "Signals", "Tube's own failures"). Trap 13.

**Files**: `internal/core/sandbox/sandbox.go`, `keeper.go`, `protocol.go`, `lookpath.go`, tests; `internal/core/pty/pty.go`, `pty_test.go`.

**Tasks**

1. `keeper.go`:
   - At its start the keeper calls `signal.Notify(ch, syscall.SIGINT, syscall.SIGQUIT)` and drops what arrives (a goroutine that only reads the channel): the terminal sends these to the application directly. It does the same for SIGTERM and SIGHUP, but passes those on to the application when one runs.
   - The `signal` message: send that signal to the application (`cmd.Process.Signal`); with no application, answer nothing.
   - When the socket closes while the application runs: send SIGTERM to the application, wait for it for the kill delay of the `run` message, then SIGKILL, wait for it, and return 0. When the socket closes with no application: return 0.
2. `sandbox.go`: `func (s *Sandbox) Signal(sig syscall.Signal) error` sends the `signal` message (it needs no answer). After `exited` it does nothing and returns nil. `Close`, while the application runs, now ends it through the keeper (closing the socket does it); `Close` waits for the keeper for the kill delay plus the step's timeout before it kills the keeper.
3. `lookpath.go`:
   ```go
   // LookPath finds the application in PATH, as a shell would, and returns its absolute path.
   func LookPath(name string) (string, error)
   // ExitCodeOf is the exit status that Tube ends with for an error of starting the
   // application: 127 when it was not found, 126 when it cannot be executed, else 125.
   func ExitCodeOf(err error) int
   ```
   `LookPath` uses `exec.LookPath` and `filepath.Abs`; a name that is not found is `SandboxLookPathFailed` (exported constant, metadata `"name"` made safe with `sanitize.Display(name, 255)`). `ExitCodeOf` returns 127 for `SandboxLookPathFailed` and `SandboxAppNotFound`, 126 for `SandboxAppNotExecutable`, 125 for everything else.
4. `internal/core/pty/pty.go` (Linux only), a pseudo-terminal:
   ```go
   // Open makes a pseudo-terminal and returns its two sides: what is written to one is read
   // from the other, through the terminal's line discipline.
   func Open() (master, slave *os.File, err error)
   // SetSize tells the terminal its size.
   func SetSize(f *os.File, cols, rows int) error
   ```
   `Open`: `unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)`; unlock it with `unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0)`; its number is `unix.IoctlGetInt(fd, unix.TIOCGPTN)`; the slave side is `/dev/pts/<number>`, opened with `os.OpenFile(path, os.O_RDWR|syscall.O_NOCTTY, 0)`. Make the master side non-blocking (`unix.SetNonblock`) before `os.NewFile`, so that it has deadlines. `SetSize`: `unix.IoctlSetWinsize(int(f.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Row: ..., Col: ...})`. Tags `PtyOpenFailed`, `PtyUnlockFailed`, `PtyNumberFailed`, `PtySlaveOpenFailed`, `PtySizeFailed`. Confirm the names in the source of `x/sys/unix`. A read of the master side fails with `EIO` once every program that had the slave side open is gone: callers treat that as the end.
5. `sandbox.Options` gets `Terminal bool`: the application becomes the leader of a new session, and `Stdin` (which must then be a terminal, the slave side of a `pty`) becomes its controlling terminal. The `run` message carries it (`terminal`), and the keeper adds `Setsid: true, Setctty: true, Ctty: 0` to the application's `SysProcAttr` (`Ctty` is the descriptor's number in the application: 0 is its standard input). Without `Terminal` nothing changes: the application stays in Tube's session and process group, on Tube's terminal.

**Tests** (inside the namespace unless said; the kill delay is 500 ms in these tests)

- `TestSignalReachesTheApplication`: the application `sh -c 'trap "exit 42" USR1; while :; do sleep 0.1; done'`; after `Run`, wait 300 ms, `Signal(SIGUSR1)`; `Wait` is code 42.
- `TestSigtermEndsTheApplication`: the application `sleep 30`; `Signal(SIGTERM)`; `Wait` is `Signal: SIGTERM`.
- `TestTheSocketClosingEndsEverything`: the application is the probe `stubborn` (it ignores SIGTERM with `signal.Notify` and a goroutine that drops it, writes its pid into the file named by a variable, and sleeps); read the pid; `Close()`; within the kill delay plus 2 seconds the pid is gone, and so is the keeper.
- `TestTheKeeperSurvivesCtrlC`: the application `sleep 30`; send SIGINT to the keeper's pid (the test knows it through `export_test.go`); 300 ms later the keeper still runs and the application still runs; then `Close`.
- `TestTheApplicationGetsDefaultSignals`: the application is the probe `sigint`, which prints whether SIGINT is ignored in it (`signal.Ignored(syscall.SIGINT)`): it prints `false`. This is trap 13 as a test.
- `TestAppNotFound`: `Options.Path` is `/nonexistent/app`: `Run` fails with `SandboxAppNotFound`, `ExitCodeOf(err)` is 127. `TestAppNotExecutable`: a file of mode 0600 with text in it: `SandboxAppNotExecutable`, 126. A folder as the path: 126.
- `pty_test.go`: `TestOpen`: text written to the slave side is read from the master side, and a line written to the master side is read from the slave side; `TestSetSize`: the size is read back with `unix.IoctlGetWinsize` on the slave side; `TestTheEnd`: after the slave side is closed, a read of the master side fails, and the test names the error it saw.
- `TestTheApplicationOnItsOwnTerminal`: `Options.Terminal` with the slave side of a `pty` as the three streams; the application `sh -c 'tty; echo hello; sleep 30'`; read the master side until `hello` (the line before it is a `/dev/pts/` path); write the byte `0x03` to the master side (what Ctrl-C types); `Wait` is `Signal: SIGINT` (status 130). The terminal did the work: no signal was sent by the test.
- `TestLookPath` (outside the namespace): `sh` gives an absolute path; `tube-no-such-program` is `LookPath.SandboxLookPathFailed`; a relative path with a slash to a file of the test is made absolute.
- `TestExitCodeOf`, a table.
- After every test of this file a helper checks that no child of the test process is left (`/proc/<pid>/task/<pid>/children` of the test's own pid is empty, read after a short wait).

**Finish**: rules 10 to 15.

### Step 2.6: The pump

**Goal**: packets move between the device and the tunnel, in both directions, until the tunnel fails or the pump is stopped.

**Read**: SPEC §9.

**Files**: `internal/core/pump/pump.go`, `pump_test.go`, `pump_ns_test.go`.

**Tasks**

1. `pump.go`:
   ```go
   // Device is the TUN device, or a fake of it: one IP packet for each Read and each Write.
   type Device interface {
       Read(packet []byte) (int, error)
       Write(packet []byte) (int, error)
       SetReadDeadline(t time.Time) error
   }
   // Counters say what a pump moved.
   type Counters struct {
       ToTunnel, FromTunnel struct{ Packets, Bytes uint64 }
       Dropped              uint64 // packets that the device or the tunnel refused
   }
   type Pump struct { /* ... */ }
   // Start starts moving packets. mtu is the tunnel's.
   func Start(device Device, tunnel vpn.Tunnel, mtu int) *Pump
   // Done is closed when the pump has ended by itself: the tunnel or the device failed.
   func (p *Pump) Done() <-chan struct{}
   // Err is why it ended by itself; nil before that, and nil after Stop.
   func (p *Pump) Err() error
   // Counters returns what the pump moved so far.
   func (p *Pump) Counters() Counters
   // Stop ends the pump, waits for its goroutines, and returns the counters. It closes neither
   // the device nor the tunnel. It can be called twice.
   func (p *Pump) Stop() Counters
   ```
2. Two goroutines:
   - **device to tunnel**: a buffer of `mtu + 64` bytes; `device.Read`; `tunnel.Write`. A `Write` that fails with `vpn.KindOf(err) == vpn.KindPacketTooLarge` is counted as dropped; any other failure of `Write` ends the pump with that error, untouched. A `Read` that fails ends the pump with `errors.NewErr("PumpDeviceReadFailed", err)`, unless the pump is stopping.
   - **tunnel to device**: a buffer of 65535 bytes; `tunnel.Read`; `device.Write`. A `Read` that fails ends the pump with that error, untouched (`io.EOF` when the server left), unless the pump is stopping. A `Write` that fails with `EINVAL`, `EIO`, `ENOBUFS` or `EAGAIN` (`errors.Is`) is counted as dropped; another failure ends the pump with `errors.NewErr("PumpDeviceWriteFailed", err)`.
   - The first error wins (a `sync.Once`); then the other goroutine is interrupted, as in `Stop`.
3. `Stop`: set a flag (an `atomic.Bool`), set both read deadlines to a time in the past (`device.SetReadDeadline`, `tunnel.SetReadDeadline`), wait for both goroutines, then set both deadlines back to the zero time, so that the device and the tunnel can be used again. An `os.ErrDeadlineExceeded` while the flag is set is the stop, not a failure. The counters are `atomic.Uint64` values read into the struct.

**Tests**

- `pump_test.go`, with a fake device of the test file (two channels of packets and a deadline, like `vpntest.Tunnel`) and `vpntest.NewTunnel`:
  - `TestPacketsGoBothWays`: three packets each way arrive unchanged and in order; the counters have the packets and the bytes, from `Counters()` while the pump runs and from `Stop()`.
  - `TestTheTunnelRefusesALargePacket`: a packet over the MTU from the device is counted as dropped, and the next packet still goes.
  - `TestTheDeviceRefusesAPacket`: the fake device's `Write` returns `syscall.EINVAL` once: dropped, and the pump goes on.
  - `TestTheTunnelFails`: `tunnel.Fail(io.EOF)`: `Done` is closed within a second and `Err()` is `io.EOF` (`errors.Is`); with an `*errors.Error` of the Tag `PingTimeout`, `Err()` is that same value.
  - `TestTheDeviceFails`: the fake's `Read` returns `syscall.EBADF`: `Err()` has the route `…PumpDeviceReadFailed`; a `Write` that returns `syscall.EBADF`: `PumpDeviceWriteFailed`.
  - `TestStop`: `Stop` returns within a second, `Err()` is nil, a second `Stop` returns the same counters, the tunnel is not closed (`tunnel.Closed()` is false), and no goroutine of the pump is left (compare `runtime.NumGoroutine()` before the start and after the stop, with a short wait).
  - `TestASecondPumpOnTheSameDevice`: after `Stop`, a new pump on the same fake device and a new tunnel moves packets.
- `pump_ns_test.go` (`nstest.Require`; `TestMain` as trap 9 says): a sandbox configured with `10.9.0.2/24`, a `vpntest.Tunnel` whose `OnWrite` answers echo requests with `packettest.EchoReply4`, a pump between the sandbox's device and that tunnel, and the application `ping -c 2 -W 2 10.9.0.1` (skips without `ping`): status 0, and the counters show at least two packets each way.

**Finish**: rules 10 to 15.

### Step 2.7: The session

**Goal**: one call runs an application through a tunnel from start to end and cleans up. The refusals of SPEC §14 are tested.

**Read**: SPEC §10 (not "Reconnecting"), §14, §16.2, §8.4 (the warning).

**Files**: `internal/core/session/session.go`, `session_test.go`, `refusal_test.go`, `session_server_test.go` (tag `tube_server`).

**Tasks**

1. `session.go`:
   ```go
   // Options say what a session runs and with what.
   type Options struct {
       Tunnel      vpn.Tunnel   // connected; the session closes it
       Dirs        storage.Dirs // for the temporary folder
       Path        string       // the application, absolute (sandbox.LookPath)
       Argv        []string     // its argv, argv[0] included
       ProfileName string       // for TUBE_PROFILE and the lines
       DNS         []netip.Addr // the profile's own DNS servers, or none
       Logger      *slog.Logger // nil logs nothing
       Lines       io.Writer    // where Tube's lines go; nil writes none (--quiet)
       Clock       clock.Clock  // nil is the system's
       ExtraEnv    []string     // more variables for the application ("NAME=value"); one replaces a variable of the same name
       Sandbox     sandbox.Options // only Exe, StepTimeout, KillDelay, Stdin, Stdout, Stderr, Terminal and Dir are read
   }
   type Session struct { /* ... */ }
   // Start makes the namespace, configures it, starts the pump and the application.
   func Start(ctx context.Context, opts Options) (*Session, error)
   // Wait returns when the application has ended, with the exit status of SPEC §10, and cleans up.
   func (s *Session) Wait() (int, error)
   // Signal passes a signal to the application.
   func (s *Session) Signal(sig syscall.Signal)
   ```
2. `Start`, in this order; when a step fails, everything made before it is undone (the pump stopped, the sandbox closed, the tunnel closed, the temporary folder removed) and the error is returned:
   1. `opts.Dirs.NewTempDir(ctx)`.
   2. `sandbox.Start` with the path, the argv, the temporary folder, and the environment `os.Environ()` plus `TUBE=1`, `TUBE_PROFILE=<sanitize.Display(name, 64)>`, `MOZ_NO_REMOTE=1`, then `opts.ExtraEnv` (a variable that is already in the environment is replaced, not doubled).
   3. `Configure` with `opts.Tunnel.Network()` and `opts.DNS`. Each warning is written as a line (`tube: <warning>`) and logged.
   4. `pump.Start(sandbox.Device(), tunnel, network.MTU)`.
   5. `sandbox.Run`.
   6. The line `tube: connected to <name>; running <argv[0]>` and a log record with the addresses and the DNS servers.
   7. Start one goroutine that watches the pump: when `pump.Done()` is closed while the application runs, write the line `tube: the VPN connection ended (<reason>); the application has no network` and log the error. The reason is a few words made from `vpn.KindOf(err)` (`the server did not answer`, `the server closed the connection`, ...), never the error's text. (Step 4.1 replaces this with reconnecting.)
3. `Wait`: `sandbox.Wait()`; then, whatever it returned: stop the watcher, `pump.Stop()`, log the counters, `tunnel.Close()`, `sandbox.Close()`, remove the temporary folder (`os.RemoveAll`). It returns `status.Status()`. When `sandbox.Wait` failed (the keeper was lost), it returns 125 and that error. `Wait` can be called once; a second call returns the same values.
4. `Signal` calls `sandbox.Signal` and ignores its error (the application may have ended).

**Tests** (`TestMain` as trap 9; `nstest.Require`; `storagetest.Isolate`; the tunnel is a `vpntest.Tunnel` that answers pings through `OnWrite`)

- `session_test.go`:
  - `TestARunFromStartToEnd`: the application `ping -c 1 -W 2 10.9.0.1` (skips without `ping`): `Wait` is 0; afterwards the tunnel is closed, the temporary folder base has no `tube-*` folder, and no child process is left.
  - `TestTheExitStatus`: `sh -c 'exit 5'` is 5; `sh -c 'kill -KILL $$'` is 137.
  - `TestTheEnvironment`: the application `sh -c 'echo $TUBE $TUBE_PROFILE $MOZ_NO_REMOTE'` with the profile name `work`: the output is `1 work 1`. With `TUBE_PROFILE=other` already in the test's environment, it is still `work`, once.
  - `TestTheLines`: with a buffer as `Lines`: it has `tube: connected to work; running sh`; with a tunnel that has no DNS server and no DNS in the options, it also has the warning; with `Lines` nil nothing is written anywhere (the sandbox's `Stderr` file is empty).
  - `TestStartFailsCleanly`: `Path` is `/nonexistent/app`: `Start` fails, `sandbox.ExitCodeOf(err)` is 127, the tunnel is closed, no temporary folder and no process is left.
  - `TestTheTunnelFailsWhileTheApplicationRuns`: the application `sleep 1`; `tunnel.Fail(io.EOF)` after `Start`: the line about the VPN is written, the application still ends by itself, `Wait` is 0.
  - `TestSignal`: `sleep 30`, `Signal(SIGTERM)`: 143.
- `refusal_test.go` (the promises of SPEC §14; each is one test):
  - `TestNoNetworkBeforeThePump`: `sandbox.Start` and `Configure` by hand, with a peer (`sandboxtest.NewPeer`) that answers nothing, **no pump**: the application `ping -c 1 -W 1 10.9.0.1` fails. (The packets reach the device and nobody moves them on.)
  - `TestNoNetworkAfterTheTunnelEnded`: a session; the application is the probe `ping-twice`, which pings once, signals through a file that it did, waits for another file, and pings again; between the two the test makes the tunnel fail. The first ping works and the second fails; the probe prints both results.
  - `TestNothingButTheTunnelAndTheLoopback`: the probe `report` in a session: the links are `lo` and `tube0` only.
  - `TestNoNetworkAfterTubeIsKilled`: the test starts a helper process (the test binary with the variable `TUBE_SESSION_HOLDER=1`, handled in `TestMain`), which starts a session with a fake tunnel and the probe `linger` as the application, with a kill delay of 3 seconds. `linger` ignores SIGTERM, writes `ready` into a file, then looks every 50 ms whether `tube0` still exists (`net.InterfaceByName`), and when it is gone writes the names of the links that are left into a second file and exits. The test waits for `ready`, kills the helper with SIGKILL, and reads the second file within 3 seconds: it has `lo` only. Then it waits until `linger` and the keeper are gone.
  - `TestTheApplicationCannotChangeTheNetwork`: the probe `report`: `add_route_error` is not empty.
  - `TestASecretStaysOutOfTheLog`: a session with a logger that writes to a buffer and a profile name that holds a marker made of an escape sequence: the log and the lines have no byte below 0x20 but the new line.
- `session_server_test.go` (tag `tube_server`): `TestARunThroughARealServer`: `vpntest.StartServer`, add its profile, `vpn.Dial`, `session.Start` with `ping -c 2 -W 2 10.8.0.1`: 0. `TestIPv6ThroughARealServer`: with `PushIPv6`, `ping -c 1 -W 2 fd00:8::1`: 0.

**Finish**: rules 10 to 15. **Last step of M2**: `devVersion` becomes `0.4.0-dev`; tag `v0.3.0` with the message `M2: The namespace`; push.

---

## M3: The command line (v0.4.0)

### Step 3.1: Connecting and explaining

**Goal**: `app.App` opens Tube's folders and log, finds a profile by name, connects it, and runs the application; `app.Explain` turns an error into a sentence.

**Read**: SPEC §12 (the events come in step 5.1), §12.3, §7, §11 (how `--profile` finds a profile), §15.

**Files**: `internal/app/app.go`, `explain.go`, with their tests; `internal/app/app_server_test.go` (tag `tube_server`).

**Tasks**

1. `app.go`:
   ```go
   // Credentials are the user name and the password that the user gave.
   type Credentials struct{ Username, Password string }

   // Options say how the application is opened.
   type Options struct {
       Env   storage.Env // nil is os.LookupEnv
       Argv  []string    // the application and its arguments; empty when Tube only keeps the profiles
       Lines io.Writer   // where Tube's lines go while the application runs; nil for --quiet
       Clock clock.Clock // nil is the system's
   }
   type App struct { /* dirs, store, logger and its closer, clock, the connected tunnel, the session */ }
   // Open resolves and makes the folders, opens the log and the store.
   func Open(ctx context.Context, opts Options) (*App, error)
   // Close closes what is open: a tunnel that was connected and never run, and the log.
   func (a *App) Close() error
   // Target is the application that will run: its name and arguments, or none.
   func (a *App) Target() []string
   // FindProfile finds a usable profile by its name, as --profile does (SPEC §11).
   func (a *App) FindProfile(ctx context.Context, name string) (id string, needsPassword bool, username string, err error)
   // Connect connects the profile. With remember, the user name of the credentials is saved
   // as the profile's. After it returned nil, Run runs the application.
   func (a *App) Connect(ctx context.Context, id string, creds *Credentials, remember bool) error
   // Run runs the application through the connected VPN and returns its exit status.
   func (a *App) Run(ctx context.Context) (int, error)
   // Signal passes a signal to the application, when one runs.
   func (a *App) Signal(sig syscall.Signal)
   ```
   - `Open`: `storage.Resolve`, `Ensure`, `logging.Open(ctx, filepath.Join(dirs.State, "tube.log"), slog.LevelInfo)`, `profiles.NewStore`. It logs one record, `tube started`, with the version.
   - `FindProfile`: list the store; among the profiles that are not broken, the one whose name is equal; else, when exactly one is equal without case (`strings.EqualFold`), that one. None: `AppProfileNotFound` (exported constant, metadata `"name"`, made safe). Several without case and none exact: `AppProfileAmbiguous`. A profile that is found but broken: `AppProfileBroken` (metadata `"reason"`).
   - `Connect`: `store.Get`; `vpn.Dial` with the credentials turned into `vpn.Credentials`; on success keep the tunnel, `store.MarkUsed`, and with `remember` and credentials, `store.SetUsername`. A failure of those two after a good dial is logged and not returned: the connection is what the user asked for. Calling `Connect` while a tunnel is kept closes the old one first. Every failure is logged with `logger.Error("connect failed", "err", err, "profile", id)`.
   - `Run`: with no tunnel, `AppNotConnected`; with no target, `AppNoTarget`. `sandbox.LookPath(argv[0])`, then `session.Start` with the tunnel, the profile's name and DNS servers, the logger, the lines, the clock, and `Wait`. The tunnel now belongs to the session. An error of `LookPath` or `Start` closes the tunnel.
2. `explain.go`:
   ```go
   // Explain is the sentence for the user about an error (SPEC §12.3). It never contains a secret.
   func Explain(err error) string
   // ExitCode is the exit status that Tube ends with after an error of its own (SPEC §10).
   func ExitCode(err error) int
   ```
   `Explain` looks, in this order, at: `profiles.ProblemOf(err)` (when its kind is not `ProblemNone`), `vpn.KindOf(err)` (when it is not `KindOther`), the Tags of Tube that are listed below, and at last the error's own message. After the sentence comes the Tag in parentheses, when the error has one: `The VPN server refused the user name or the password. (AuthFailed)`. The sentences (the words in braces come from the metadata, already made safe; when a piece is missing, leave its part of the sentence out):

   | What | Sentence |
   |---|---|
   | `ProblemUnsupported` | `The profile uses the option "{option}" on line {line}, which Tube does not support: {reason}.` |
   | `ProblemBadOption` | `The option "{option}" on line {line} of the profile is not valid: {reason}.` |
   | `ProblemMissing` | `The profile has no "{option}".` |
   | `ProblemFile` | `A file that the profile names with "{option}" cannot be used. Put it next to the profile, or inside the profile.` |
   | `ProblemSyntax` | `The profile cannot be read: a mistake on line {line}.` |
   | `ProblemTooLarge` | `The profile is too large.` |
   | `ProblemUnreadable` | `The profile cannot be read.` |
   | `KindAuthFailed` | `The VPN server refused the user name or the password.` |
   | `KindNoAnswer` | `The VPN server did not answer.` |
   | `KindCertificate` | `The certificate of the VPN server was not accepted.` |
   | `KindServerEnded` | `The VPN server closed the connection.` |
   | `KindUnsupported` | `The VPN server asks for something that Tube does not support.` |
   | `KindCancelled` | `Cancelled.` |
   | `vpn.TagCredentialsNeeded` | `This profile needs a user name and a password.` |
   | `profiles.TagProfileRunsScripts` | `This profile runs scripts ({options}). Tube never runs them.` |
   | `profiles.TagProfileNameTaken` | `Another profile is already called "{name}".` |
   | `ProfileNameEmpty` | `A profile needs a name.` |
   | `ProfileNameTooLong` | `The name is too long (64 characters at most).` |
   | `ProfileFileTooLarge` | `The file "{file}" of the profile is too large.` |
   | `ProfileDNSInvalid` | `"{value}" is not an IP address.` |
   | `ProfileDNSTooMany`, `ProfileTooManyDNS` | `A profile can have 4 DNS servers at most.` |
   | `AppProfileNotFound` | `There is no profile called "{name}".` |
   | `AppProfileAmbiguous` | `More than one profile is called "{name}" when the case is ignored. Write the name exactly.` |
   | `sandbox.TagUserNamespaceDenied` | `This machine does not let an ordinary user make a user namespace, which Tube needs. See docs/GUIDE.md ("What Tube needs").` |
   | `sandbox.TagTunUnavailable` | `Tube cannot open /dev/net/tun.` |
   | `sandbox.TagLookPathFailed`, `sandbox.TagAppNotFound` | `The program "{name}" was not found.` |
   | `sandbox.TagAppNotExecutable` | `The program cannot be executed.` |
   | anything else | the error's message |

   Export the constants this table names from their packages when they are not exported yet. `ExitCode` is `sandbox.ExitCodeOf(err)`.

**Tests**

- `TestExplain`, a table with one row for every row above: the error is made the real way where that is short (parse a broken profile, `store.Rename` to a taken name), else as `&errors.Error{Tag: ..., Metadata: ...}`. Compare the whole sentence.
- `TestExplainKeepsSecretsOut`: an error whose metadata has `"password": "hunter2"` and whose message has `hunter2`: for a Tag of the table the sentence does not contain it. (For an unknown Tag the message is shown: so no code of Tube may put a secret into a message; `TestSecretsStayOut` of step 1.1 and the tests of step 1.4 keep that.)
- `TestExitCode`.
- `TestOpen`: with `storagetest.Isolate`: the folders exist with mode 0700, `tube.log` exists with mode 0600 and has the record `tube started`. With `HOME` unset and no `TUBE_` variable (give an `Env` of the test): the error of `storage.Resolve`.
- `TestFindProfile`, a table over a store with `Work`, `work2`, `HOME` and `home` and one broken profile: `Work` (exact), `work` (one match without case), `home` (exact wins over `HOME`), `Home` (ambiguous), `nothing` (not found), the broken one's name (`AppProfileBroken`).
- `TestConnectNeedsCredentials`: a profile with `auth-user-pass`, nil credentials: `Connect` returns the error of `vpn.Dial` (`vpn.TagCredentialsNeeded`), and the log has the record `connect failed` without any password in the file.
- `TestRunWithoutConnect`: `AppNotConnected`. `TestRunWithoutTarget`: after a connect that a test fakes (an unexported method that sets the tunnel to a `vpntest.Tunnel`): `AppNoTarget`.
- `TestRunWithAFakeTunnel` (`nstest.Require`, `TestMain` as trap 9): the fake tunnel set as above, the target `sh -c 'exit 4'`: `Run` returns 4; the log has the start and the end of the run.
- `app_server_test.go` (tag): `TestConnectAndRun`: the server's profile added, `Connect`, `Run` with `ping -c 1 -W 2 10.8.0.1`: 0, and `LastUsed` of the profile is set. `TestConnectRemembersTheUserName`: a server with a password; `Connect` with `remember`: the profile's `Username` is the one given. `TestAWrongPassword`: `Explain(err)` is the sentence of `KindAuthFailed`.

**Finish**: rules 10 to 15.

### Step 3.2: `tube --profile`

**Goal**: `tube --profile <name> <application> [arguments]` works from the command line, end to end, without a TUI.

**Read**: SPEC §11, §10 ("Signals", "Tube's own failures", the lines), §8.5 (the last paragraph), §8.8. Trap 13.

**Files**: `cmd/tube/main.go`, `flags.go`, `password.go`, `usage.go`, with tests; `cmd/tube/e2e_server_test.go` (tag `tube_server`); `internal/core/vpn/vpntest/dns.go` (tag `tube_server`); `docs/MANUAL-CHECKS.md`.

**Tasks**

1. `usage.go`: the constant `usage`, the text of SPEC §11 (the two forms and the options), ending with a new line.
2. `flags.go`:
   ```go
   type flags struct {
       profile  string
       authFile string
       quiet    bool
       version  bool
       help     bool
       target   []string // the application and its arguments
   }
   func parseFlags(args []string) (flags, error)
   ```
   Use `flag.NewFlagSet("tube", flag.ContinueOnError)` with `SetOutput(io.Discard)`; register each option under both names with the same variable (`fs.StringVar(&f.profile, "profile", "", "")` and `fs.StringVar(&f.profile, "p", "", "")`; `quiet` and `q`; `help` and `h`; `auth-file`; `version`). The `flag` package stops at the first argument that is not an option and at `--`, and accepts `-profile` and `--profile` alike: that is what SPEC §11 wants. `fs.Args()` is the target. Errors, all with exit status 2: an option that is not known or has no value (`TubeUsage`, wrapping the `flag` error); `--profile` with no target (`TubeProfileWithoutApplication`); `--auth-file` without `--profile` (`TubeAuthFileWithoutProfile`).
3. `password.go`:
   ```go
   // readAuthFile reads the user name (first line) and the password (second line) of a file
   // that only the user can read.
   func readAuthFile(path string) (app.Credentials, error)
   // askCredentials asks on the terminal for the user name (when username is empty) and for
   // the password, which is not echoed.
   func askCredentials(tty *os.File, username string) (app.Credentials, error)
   ```
   - `readAuthFile`: `os.Stat`; a mode with any bit of `0o077` set is `TubeAuthFileTooOpen` (the message says `chmod 600`); read at most 4 KiB; two lines are needed (`TubeAuthFileIncomplete`); a `\r` at the end of a line is dropped. Tags `TubeAuthFileUnreadable` for the other failures.
   - `askCredentials`: write `User name: ` and read a line, when needed; write `Password: `; switch the echo off (`unix.IoctlGetTermios(fd, unix.TCGETS)`, clear `unix.ECHO` in `Lflag`, `unix.IoctlSetTermios(fd, unix.TCSETS, &changed)`); read a line; **always** switch it back (a `defer` that sets the saved value), and write a new line. Read the line byte by byte until `\n`, so that nothing after it is taken. Tags `TubeTerminalFailed` (the termios calls), `TubeTerminalReadFailed`, `TubeTerminalClosed` (the end of input before a line).
   - The caller opens `/dev/tty` (`os.OpenFile("/dev/tty", os.O_RDWR, 0)`); when that fails: `TubeNoTerminal` ("the profile needs a password and there is no terminal to ask on; use --auth-file").
4. `main.go`: `main` is: the keeper check; then `os.Exit(run(context.Background(), os.Args[1:], os.LookupEnv, os.Stdout, os.Stderr))`.
   ```go
   func run(ctx context.Context, args []string, env storage.Env, stdout, stderr io.Writer) int
   ```
   In this order:
   1. `parseFlags`. `--help`: print `usage` on standard output, 0. `--version`: `tube <version>`, 0. An error: `tube: <app.Explain(err)>` and the line `Try 'tube --help'.` on standard error, 2.
   2. With a target: `sandbox.LookPath(target[0])`; an error is printed and the status is 127. (The path is found again in `App.Run`; here it is only the early check of SPEC §8.5.)
   3. `sandbox.Check(ctx)`; an error is printed, 125.
   4. `app.Open` with the target and with `Lines` set to standard error, or nil with `--quiet`. `defer` its `Close`.
   5. Without `--profile`: print `tube: the TUI comes with milestone M5; use --profile <name>` on standard error, 125. (Step 5.2 replaces this.)
   6. With `--profile`: `FindProfile`; when the profile needs a password: `readAuthFile`, or `askCredentials` on `/dev/tty` with the remembered user name; `Connect` (with `remember` false: a script does not change the profile). The context of `Connect` is cancelled by SIGINT, SIGTERM and SIGHUP (`signal.NotifyContext`), and a cancelled connect ends with status 130 and no message.
   7. `App.Run`. While it runs: `signal.Notify(ch, SIGINT, SIGQUIT, SIGTERM, SIGHUP)` and a goroutine that drops SIGINT and SIGQUIT and gives SIGTERM and SIGHUP to `App.Signal` (trap 13: `Notify`, not `Ignore`). Stop the notification when `Run` returned.
   8. The status is `Run`'s. An error of Tube's own at any point: `tube: <app.Explain(err)>` on standard error, and `app.ExitCode(err)`.

   Every error message is one line, starts with `tube: `, and goes to standard error; standard output belongs to the application (and to `--help` and `--version`).
5. `vpntest/dns.go` (tag): `func StartDNS(t testing.TB, addr netip.AddrPort, names map[string]netip.Addr)`: a UDP listener on the address (`net.ListenUDP`), answering with `dnstest.Answer`, stopped by `t.Cleanup`. The tests use it on the server's own tunnel address, port 53 (the test is root of its namespace, so it may bind it).
6. `docs/MANUAL-CHECKS.md` (create it): a short header that says what the file is (CLAUDE.md "Working through the steps"), then the first item, with the step's id: `tube --profile <a real profile> curl -s https://ifconfig.me` prints the VPN's address and not the machine's; `curl -s https://ifconfig.me` beside it prints the machine's.

**Tests**

- `flags_test.go`, a table: no arguments; `firefox`; `ping -c 3 host` (the `-c` belongs to the application); `-p work ping`; `--profile work -- ping -c 1 host`; `--profile=work ping`; `-q -p work ping`; `--version`; `-h`; `--nosuch` (`TubeUsage`); `--profile` alone (`TubeUsage`: no value); `--profile work` (`TubeProfileWithoutApplication`); `--auth-file f ping` (`TubeAuthFileWithoutProfile`).
- `password_test.go`: the auth file: good; one line; a missing file; mode 0644 (refused, by its route); Windows line ends. The terminal, through a pseudo-terminal (`pty.Open()` of step 2.5): write `alice\nsecret\n` to the master side; `askCredentials(<the slave side>, "")` returns them; what the master side reads back has `User name: ` and `Password: ` and does **not** have `secret` (the echo was off); after the call the echo is on again (read the termios of the slave). With a user name given, only the password is asked. A master that is closed before the line: `TubeTerminalClosed`.
- `main_test.go`: `run` with `--help`, `--version`, a wrong option (2, with the hint), a program that does not exist (127), no `--profile` (125 with the sentence of task 4.5), `--profile nothing true` in an isolated store (125 and `There is no profile called "nothing".`). Use `storagetest.Isolate` and pass `os.LookupEnv`.
- `e2e_server_test.go` (tag; the binary is built once for the file with `go build -o <temp>/tube ./cmd/tube` from the test, and started with `exec.Command` under a context of 60 seconds, with an environment the test builds: the isolated folders, `PATH`, `HOME` of the test; the profile is added to the same configuration folder with `profiles.NewStore(...).Add` in the test's process):
  - `TestPing`: `tube --profile test ping -c 2 -W 2 10.8.0.1`: 0; standard error has `tube: connected to test`.
  - `TestQuiet`: with `--quiet`, standard error is empty.
  - `TestTheExitStatus`: `tube --profile test sh -c 'exit 7'`: 7.
  - `TestTheNamesGoToTheVPN`: the server pushes the DNS server `10.8.0.1`; `StartDNS` there with `tube-test.example` at `192.0.2.7`; `tube --profile test getent hosts tube-test.example` (skips without `getent`): 0 and the address in the output.
  - `TestAWrongPassword`: a server with a password, an auth file with a wrong one: 125, and standard error is `tube: The VPN server refused the user name or the password. (AuthFailed)`; the right one: 0.
  - `TestTheApplicationIsNotFound`: `tube --profile test tube-no-such-program`: 127, before any connection (the server's log has no new client).
  - `TestSigterm`: `tube --profile test sleep 30`; after the line `connected`, SIGTERM to Tube: it ends with 143 within 10 seconds, and no `sleep` of the test is left.
  - `TestNothingIsLeft`: after the tests of the file, no process whose command line has the test's temporary folder in it is left.

**Finish**: rules 10 to 15. **Last step of M3**: `devVersion` becomes `0.5.0-dev`; tag `v0.4.0` with the message `M3: The command line`; push.

---

---

## M4: A session that changes (v0.5.0)

### Step 4.1: Connect again

**Goal**: when the VPN connection ends while the application runs, Tube connects again by itself; in between, the application has no network. This works with `--profile` now, and in the TUI from M6 on.

**Read**: SPEC §10 ("Reconnecting"), D14, §14.

**Files**: `internal/core/session/session.go`, `reconnect.go`, with tests; `internal/app/app.go`; `cmd/tube/e2e_server_test.go`.

**Tasks**

1. `session.Options` gets `Redial func(ctx context.Context) (vpn.Tunnel, error)`: when it is nil, the session behaves as in step 2.7.
2. `reconnect.go`: the watcher of step 2.7 becomes a loop, in one goroutine that owns the tunnel and the pump: it waits in a `select` for `pump.Done()`, for the timer of a wait, for the result of a redial, and for the application's end. (Step 4.2 adds the user's commands to the same `select`; write the loop so that another case is easy to add.) When `pump.Done()` is closed while the application runs:
   1. `pump.Stop()`; close the old tunnel; write `tube: the VPN connection ended (<reason>); the application has no network; connecting again…`; log the error.
   2. Wait, then call `Redial`. The waits are 1, 2, 4, 8, 15 seconds and then 30 seconds each time, on the session's clock (`clock.AfterFunc` and a channel; never `time.Sleep`). The first try comes after the first wait.
   3. `Redial` failed with `vpn.KindOf(err) == vpn.KindAuthFailed`: write `tube: the VPN server refused the user name or the password; the application stays without network` and stop trying, for the rest of the session. Any other failure: log it, write nothing more (one line for the loss is enough), and go to 2 with the next wait.
   4. `Redial` worked: `sandbox.Configure` with the new network (the addresses may have changed), a new pump, the line `tube: connected again`. When `Configure` fails: close the new tunnel, log, and go to 2.
   5. The end of the application, or of the session's context, stops the loop at once: a wait is cut short, a `Redial` in flight is cancelled through its context, and a tunnel that it still returns is closed.
3. Only the loop's goroutine touches the tunnel and the pump. `Wait` stops the loop first and waits for it to end; the loop closes what is in use when it ends. Then `Wait` cleans up the rest.
4. `app.go`: `Run` gives the session a `Redial` that calls `vpn.Dial` for the same profile with the credentials of `Connect`, which the App keeps in memory until `Close` and never writes anywhere. Each try has a context of 60 seconds.

**Tests** (fake tunnels, the fake clock, `nstest.Require`)

- `TestReconnect`: a session whose application is the probe `ping-twice` (step 2.7); the first tunnel fails with `io.EOF` after the first ping; `Redial` returns a second fake tunnel (with another address, `10.10.0.5/24`); advance the clock by a second; when the line `connected again` is written, let the probe ping again (it pings the second network's peer): both pings work. The probe `report`, run in another session the same way, shows the new address and one default route.
- `TestNoNetworkBetweenTheTunnels`: as above, but the probe pings three times: before, between (the test lets it ping after the loss and before advancing the clock), after: works, fails, works.
- `TestTheDelays`: `Redial` fails five times with an `*errors.Error` of the kind `KindNoAnswer`, then works: the calls come after the clock was advanced by 1, 2, 4, 8, 15 and 30 seconds, and not before (use `clock.Fake.Pending()` to know that the session is waiting, and count the calls).
- `TestARefusedLogin`: `Redial` returns an error of the kind `KindAuthFailed`: the line is written, and `Redial` is not called again however far the clock goes; the application ends normally.
- `TestTheApplicationEndsWhileWaiting`: the application ends during a wait: `Wait` returns at once, and `Redial` is not called afterwards.
- `TestTheApplicationEndsWhileDialing`: `Redial` blocks until its context is cancelled: `Wait` returns within a second.
- `TestWithoutRedial`: nil: the behavior of step 2.7 (its test still passes).
- `e2e_server_test.go` (tag): `TestTheServerRestarts`: `tube --profile test ping -i 1 -c 25 -W 1 10.8.0.1`; after three replies in the output, `server.Stop()`; three seconds later `server.Start()`; the output gets replies again, with numbers after the loss, standard error has `the VPN connection ended` and `connected again`, and the process ends by itself. (The server pushes `keepalive 1 4`, so the client notices the loss within about 4 seconds, and the first redial comes a second later.)

**Finish**: rules 10 to 15.

### Step 4.2: Disconnecting, switching, and the session's status

**Goal**: while the application runs, the network can be taken away from it and another VPN can be given to it, and the session says what it is doing: its state, its tunnel, and how much traffic passed.

**Read**: SPEC §10 ("Disconnecting and switching", "Status"), §14.

**Files**: `internal/core/session/session.go`, `reconnect.go`, `control.go`, `status.go`, with tests.

**Tasks**

1. `status.go`:
   ```go
   // State is what the session's connection is doing.
   type State int
   const (
       StateConnected    State = iota // a tunnel is in use
       StateReconnecting              // the tunnel ended; Tube is connecting again
       StateDisconnected              // the user disconnected, or a switch failed: no tries
       StateRefused                   // the server refused the login when connecting again: no tries
   )
   // Status is a picture of the session at one moment.
   type Status struct {
       State       State
       Since       time.Time     // when this state began (the session's clock)
       ProfileName string        // the profile in use, or the last one
       Network     vpn.Network   // of the tunnel in use; the zero value when there is none
       DNS         []netip.Addr  // the servers that resolv.conf of the namespace has now
       Traffic     pump.Counters // everything since the session started, over all its tunnels
       AppRunning  bool
       Exit        int           // the exit status, once the application has ended
   }
   func (s *Session) Status() Status
   // Done is closed when the application has ended.
   func (s *Session) Done() <-chan struct{}
   ```
   `session.Options` gets `OnChange func()`: the session calls it after every change of `State` and when the application ends, from its own goroutine, never while it holds a lock, and never after `Wait` returned.
2. `control.go`:
   ```go
   // Connection is a tunnel and what the session needs to know with it.
   type Connection struct {
       Tunnel      vpn.Tunnel
       ProfileName string
       DNS         []netip.Addr // the profile's own DNS servers, or none
       Redial      func(ctx context.Context) (vpn.Tunnel, error) // nil: no reconnecting
   }
   // Disconnect takes the network away from the application: the pump stops, the tunnel is
   // closed, and Tube does not connect again until Use is called.
   func (s *Session) Disconnect()
   // Use gives the application this tunnel in place of the one it has, or of none.
   func (s *Session) Use(ctx context.Context, c Connection) error
   ```
   - `Disconnect`: the state becomes `StateDisconnected`; the line `tube: disconnected; the application has no network`. When the state is already `StateDisconnected` it does nothing.
   - `Use`, in this order: stop the tries of step 4.1 (a wait is cut, a redial in flight is cancelled and what it returns is closed); stop the pump, when there is one; close the old tunnel, when there is one; `sandbox.Configure` with the new network and DNS servers (write its warnings as lines); a new pump; remember the profile's name and the `Redial`; the state becomes `StateConnected`; the line `tube: now using <name>`. When `Configure` fails: close the **new** tunnel, the state becomes `StateDisconnected`, and the error is returned (fail closed: the application has no network, and the user chooses again). After the application has ended: close the given tunnel and return `SessionEnded`.
   - `session.Start` now takes its first connection the same way: `Options.Tunnel`, `ProfileName`, `DNS` and `Redial` stay as they are, and inside they become the first `Connection`.
3. The controller. `Disconnect`, `Use`, the end of the pump, the result of a redial, the end of a wait and the end of the application all change the same things (the tunnel, the pump, the state). Let **one goroutine own them**: the loop of step 4.1 becomes that goroutine, with a `select` over a channel of commands (`Disconnect`, `Use` with a channel for its answer), `pump.Done()`, the redial's result, the timer of the wait, and the application's end. No other goroutine touches the tunnel or the pump. `Status()` reads a copy that the goroutine writes under a mutex after each change; its `Traffic` is the sum of the counters of the pumps that were stopped plus `pump.Counters()` of the one that runs.
4. `Wait` stops the controller (it closes what is in use), then cleans up as before.

**Tests** (fake tunnels that answer pings, the fake clock, `nstest.Require`; the probe `ping-on-demand` pings each time a file appears and writes each result to another file, so that a test decides when the application tries the network)

- `TestDisconnect`: a ping works; `Disconnect()`; a ping fails; the old tunnel is closed; `Status().State` is `StateDisconnected`; advancing the clock by five minutes calls `Redial` zero times; the line was written once; a second `Disconnect` writes nothing.
- `TestUseAfterDisconnect`: after a disconnect, `Use` with a second tunnel on `10.10.0.5/24`: a ping to that network's peer works; `Status()` has the new network and the new profile name.
- `TestUseWhileConnected`: `Use` with a second tunnel: the first is closed; pings work through the second; the first tunnel got no packet after the switch.
- `TestUseWhileReconnecting`: the first tunnel fails; while the session waits (`clock.Fake.Pending()`), `Use` with a second tunnel: connected at once; advancing the clock calls the old `Redial` zero times.
- `TestUseWithANetworkThatCannotBeConfigured`: a tunnel whose network has an MTU of 100000 (the kernel refuses it): `Use` returns the error of `Configure`; that tunnel is closed; the state is `StateDisconnected`; a ping fails.
- `TestUseAfterTheEnd`: after the application ended: `Use.SessionEnded`, and the given tunnel is closed.
- `TestTheTrafficAddsUp`: two pings through the first tunnel, a switch, two through the second: `Status().Traffic` counts at least four packets each way.
- `TestOnChange`: the callback is called for connected to reconnecting, reconnecting to connected, a disconnect, a use, and the end of the application; never after `Wait` returned.
- `TestDone`: closed when the application ends, not before.
- `TestTheStates`: a refused login when connecting again gives `StateRefused`; a later `Use` gives `StateConnected`.
- Every test of step 4.1 and of step 2.7 still passes. Run this package with `-race -count=3` once before you finish.

**Finish**: rules 10 to 15.

### Step 4.3: The log of a terminal

**Goal**: what a program writes to a pseudo-terminal becomes plain lines that are safe to draw: the Output tab of the TUI shows them.

**Read**: SPEC §13.9, §6.5.

**Files**: `internal/core/termlog/termlog.go`, `termlog_test.go`.

**Tasks**

1. `termlog.go`:
   ```go
   // Log keeps the last lines that a program wrote to a terminal, as plain text.
   type Log struct { /* a mutex, the lines, the unfinished line, the bytes of a character or an escape sequence that are not complete yet */ }
   // New makes a log that keeps at most maxLines lines of at most maxWidth characters.
   func New(maxLines, maxWidth int) *Log
   // Write takes what the program wrote. It never fails.
   func (l *Log) Write(p []byte) (int, error)
   // Len tells which lines the log has: first is the number of the oldest line that is kept,
   // end is the number after the last. Lines are numbered from 0 since the log was made; the
   // unfinished line counts when it is not empty.
   func (l *Log) Len() (first, end int)
   // Range returns the lines from number from up to, not including, number to, as far as they
   // are kept.
   func (l *Log) Range(from, to int) []string
   // OnWrite sets a function that is called after each Write that changed the log. It is
   // called without the log's lock. Set it before the log is used.
   func (l *Log) OnWrite(f func())
   ```
2. How the bytes become lines:
   - The bytes are UTF-8. A character that is cut by the end of a `Write` is kept and finished by the next `Write`. An invalid byte becomes U+FFFD.
   - `\n` ends the line. `\r\n` is the same as `\n`.
   - `\r` alone means that what comes next overwrites the line (a progress bar): remember it, and when the next printable character comes, empty the unfinished line first.
   - Backspace (`0x08`) removes the last character of the unfinished line. A tab becomes spaces up to the next multiple of 8 columns. Every other control character (C0, DEL, C1), every Unicode format character and the line and paragraph separators are dropped.
   - Escape sequences are dropped, also when they are cut by the end of a `Write`: `ESC [` then any bytes `0x20` to `0x3F` then one byte `0x40` to `0x7E` (CSI); `ESC ]` up to `BEL` or `ESC \` (OSC); `ESC P`, `ESC X`, `ESC ^`, `ESC _` up to `ESC \`; `ESC` and one other byte. A sequence that has not ended after 256 bytes is given up: the bytes are dropped and reading goes on normally.
   - A line takes at most `maxWidth` characters; what comes after is dropped until the line ends. When there are more than `maxLines` finished lines, the oldest are dropped (and `first` grows).
3. No function of this package returns a string with a byte below `0x20`, a `0x7F`, or an escape character in it: that is the promise the TUI relies on.

**Tests** (a table of `{writes []string, want []string}` where it fits)

- Plain lines; a line without an ending (it is the last line of `Range`); `\r\n`; an empty line.
- A progress bar: `"10%\r20%\r30%\n"` is one line, `30%`. `"abc\rx\n"` is `x` (the line was emptied, not partly overwritten).
- Backspace; a tab after 3 characters makes 8 columns.
- Colors: `"\x1b[31mred\x1b[0m\n"` is `red`. A title: `"\x1b]0;title\x07text\n"` is `text`. A cursor move `"\x1b[2J\x1b[H"` gives nothing.
- A sequence cut in two writes (`"\x1b["`, then `"31mred\n"`) is `red`. A character cut in two writes (the two bytes of `é` apart) is `é`.
- A sequence that never ends (`ESC ]` and 1000 bytes without `BEL`), then `"\nok\n"`: the log has `ok` and no byte of the sequence's first 256.
- Invalid UTF-8; a bidirectional override character; a C1 control: none reaches a line.
- The width: a line of `maxWidth+10` characters is cut to `maxWidth`, and the next line is whole.
- The limit: with `maxLines` 3 and five lines, `Len()` is `(2, 5)` and `Range(0, 5)` returns the last three.
- `Range` with numbers outside what is kept returns what there is, and nothing for an empty range.
- `OnWrite` is called once for a write with text and not for an empty write.
- `TestNothingUnsafeComesOut`: write 64 KiB of random bytes (a fixed seed); no line has a byte below `0x20` or a `0x7F`, and every line is valid UTF-8.
- `TestConcurrentUse`: one goroutine writes, another calls `Range`, under the race detector.

**Finish**: rules 10 to 15. **Last step of M4**: `devVersion` becomes `0.6.0-dev`; tag `v0.5.0` with the message `M4: A session that changes`; push.

---

## M5: The TUI: the profiles (v0.6.0)

**Read this before any step of M5.** These facts about the libraries were checked for Cave (the project whose process Tube follows) on 2026-11-24 with `charm.land/bubbletea/v2` v2.1.2 and `charm.land/lipgloss/v2` v2.1.1. Use these versions. Still open the source when you use something that is not listed here.

- Import `tea "charm.land/bubbletea/v2"` and `"charm.land/lipgloss/v2"`. The module `github.com/charmbracelet/x/ansi` comes with them and may be imported under `internal/tui`.
- A model has `Init() tea.Cmd`, `Update(tea.Msg) (tea.Model, tea.Cmd)` and `View() tea.View`. Make the view with `v := tea.NewView(text)` and set `v.AltScreen = true`. Leave `v.MouseMode` at its zero value (no mouse).
- A key press is a `tea.KeyPressMsg`. Compare `msg.String()` with these names: `"a"`, `"C"` (a capital letter), `"?"`, `"/"`, `"enter"`, `"esc"`, `"tab"`, `"shift+tab"`, `"space"`, `"backspace"`, `"delete"`, `"up"`, `"down"`, `"left"`, `"right"`, `"pgup"`, `"pgdown"`, `"home"`, `"end"`, `"ctrl+c"`, `"ctrl+u"`. The text a key types is `msg.Text` (empty for keys such as Enter).
- In a test, make key presses like this: `tea.KeyPressMsg{Code: 'a', Text: "a"}`, `tea.KeyPressMsg{Code: 'C', Text: "C", Mod: tea.ModShift}`, `tea.KeyPressMsg{Code: tea.KeyEnter}`, `tea.KeyPressMsg{Code: tea.KeyEscape}`, `tea.KeyPressMsg{Code: tea.KeyTab}`, `tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}`, `tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}`.
- The size of the terminal comes as `tea.WindowSizeMsg{Width, Height}`; pasted text as `tea.PasteMsg{Content}`.
- `tea.Quit` is a command; it produces `tea.QuitMsg`. `tea.Batch(cmds...)` produces `tea.BatchMsg`, which is a `[]tea.Cmd`.
- `lipgloss.NewStyle()...Render(s)` always writes escape sequences with full colors. Bubble Tea's renderer removes what the terminal cannot show: with `NO_COLOR` set it keeps bold and reverse and drops the colors. So `NO_COLOR` needs no code of yours. What you must do: never write an escape sequence by hand, and never let color be the only sign of something (the selected row also has a `>` in front, an error also starts with `Error:`).
- Width is measured in terminal cells: `lipgloss.Width(s)` (it skips escape sequences and counts a wide character as 2). `ansi.Truncate(s, width, "…")` cuts to a width. `ansi.Strip(s)` removes the escape sequences.
- A program for a test runs without a terminal: `tea.NewProgram(model, tea.WithInput(strings.NewReader("q")), tea.WithOutput(&buffer), tea.WithWindowSize(100, 30), tea.WithoutSignalHandler())`, and `Run()` returns when the model quits.
- Running another program on the terminal (the editor): `tea.ExecProcess(cmd *exec.Cmd, callback)` gives the terminal to the program and takes it back, then sends the message that the callback returns. **Confirm its signature in the source**: it was not checked.

**How the TUI is built** (every step of M5 and M6 follows this):

- **One model, plain data.** The root `Model` holds the snapshot (read again from the app each time a `changedMsg` arrives), the size, the current page, and each page's own small state (the selected row, an open field). `View` only draws what the model holds; it never calls the app.
- **A command never runs inside `Update`.** A key that changes something returns a `tea.Cmd` that calls the app (its methods take a `ctx` and block) and returns a message with the result (`doneMsg{err error}` for most); `Update` shows `app.Explain(err)` on the message line when it arrives. So the terminal never freezes on a slow disk or a slow server.
- **No command of the model may wait.** No `tea.Tick`, no sleep. Time comes from outside: `Run` starts a goroutine with a ticker of the clock that sends `tickMsg{}` every second (for the seconds of the connecting page). This is what makes the test helper below possible.
- **The test helper** (`internal/tui/harness_test.go`, written in step 5.2) sends a message and then every message the returned commands produce, as the program would:
  ```go
  func (h *harness) send(msg tea.Msg) {
      h.t.Helper()
      queue := []tea.Msg{msg}
      for len(queue) > 0 {
          msg, queue = queue[0], queue[1:]
          switch msg := msg.(type) {
          case nil:
              continue
          case tea.QuitMsg:
              h.quit = true
              continue
          case tea.BatchMsg:
              for _, cmd := range msg {
                  if cmd != nil {
                      queue = append(queue, cmd())
                  }
              }
              continue
          }
          var cmd tea.Cmd
          h.model, cmd = h.model.Update(msg)
          if cmd != nil {
              queue = append(queue, cmd())
          }
      }
  }
  ```
  with the shortcuts `h.key("a")`, `h.keys("a", "enter")`, `h.typeText("alice")` (one key press per character), `h.paste(text)`, `h.resize(w, h)`, `h.screen() string` (`ansi.Strip` of the view's content) and `h.golden(name)`. A command that would give the terminal away (`tea.ExecProcess`) cannot run in the helper: the model makes it through a field `execEditor func(...) tea.Cmd` that the helper replaces.
- **Golden files** are text: `internal/tui/testdata/goldens/<name>.txt` holds `h.screen()`. The helper defines its own flag in a test file: `var updateGoldens = flag.Bool("update-goldens", false, "rewrite the golden files")`. Every test that compares a golden has `Golden` in its name. After an update, open every changed file and read it (trap 10).
- **Golden tests use `apptest.Sample()` or `apptest.Empty()`** (step 5.1), never a real application. Flow tests that must see a real effect use a real `app.App` over `storagetest.Isolate`.
- **The screen** (width `W`, height `H`; every line is cut to `W` cells with `…` and never wraps; the view has exactly `H` lines):
  ```
  line 0        Tube   select a profile to run firefox
  line 1        ────────────────────────────────────────────── (W cells)
  line 2…H-4    the page (H-5 lines)
  line H-3      ────────────────────────────────────────────── (W cells)
  line H-2      the message line: an error or a notice, else empty
  line H-1      the keys of the page:  enter run   a add   e open   d delete   ? keys   q quit
  ```
  Without an application, line 0 is `Tube   profiles`. Below 40 columns or 10 lines the whole view is the one line `The terminal is too small`.
- **Hostile text.** Everything from a profile is already made safe when it is stored or put into the snapshot. Draw stored strings as they are; never pass them to `fmt.Sprintf` as the format, and never build a style from them.
- **The password** is held in the login page's field and nowhere else; it is cleared when the page closes; `View` draws nothing for it, not its length either.

### Step 5.1: The snapshot and the commands

**Goal**: the `app.App` has everything the TUI calls, behind an interface, and tests of the TUI have a fake of it.

**Read**: SPEC §12.1, §12.2, §6.3, §13.5.

**Files**: `internal/app/snapshot.go`, `commands.go`, `interface.go`, with tests; `internal/app/apptest/apptest.go` and its test; `docs/SPEC.md` (§12.2, see task 5).

**Tasks**

1. `snapshot.go`:
   ```go
   // Profile is a profile as the TUI shows it. Every string is safe to draw.
   type Profile struct {
       ID            string
       Name          string
       Remotes       []string  // "vpn.example.com:1194 udp"
       NeedsPassword bool
       Username      string
       DNS           []string  // the profile's own DNS servers
       LastUsed      time.Time // the zero time: never
       Ignored       []string
       Files         []string
       Broken        string    // why it cannot be used, or ""
   }
   // Snapshot is what the TUI draws from.
   type Snapshot struct {
       Target   []string  // the application and its arguments, or none
       Profiles []Profile // sorted by name
       Now      time.Time // the clock's time, for "3 days ago"
   }
   func (a *App) Snapshot(ctx context.Context) (Snapshot, error)
   ```
   The target's strings go through `sanitize.Display(s, 255)`.
2. `commands.go`: the commands of SPEC §12.2. Each blocks, takes a `ctx`, and after a change that worked tells the subscribers.
   ```go
   // AddProfile adds the profile file at path under the name; an empty name is the file's name
   // without ".ovpn". It returns the new profile's id.
   func (a *App) AddProfile(ctx context.Context, path, name string, withoutScripts bool) (string, error)
   func (a *App) RenameProfile(ctx context.Context, id, name string) error
   func (a *App) SetUsername(ctx context.Context, id, username string) error
   // SetDNS sets the profile's DNS servers from text ("1.1.1.1, 9.9.9.9"); empty text removes them.
   func (a *App) SetDNS(ctx context.Context, id, text string) error
   func (a *App) DeleteProfile(ctx context.Context, id string) error
   // BeginEdit copies the profile's text into a file that only the user can read and returns its path.
   func (a *App) BeginEdit(ctx context.Context, id string) (path string, err error)
   // EndEdit reads the file back, replaces the profile's text when it changed, and removes the
   // file. changed says whether the profile was replaced.
   func (a *App) EndEdit(ctx context.Context, id, path string) (changed bool, err error)
   // Subscribe calls f, from any goroutine, after every change of the profiles. The returned
   // function ends the subscription.
   func (a *App) Subscribe(f func()) (cancel func())
   ```
   - `AddProfile`: a path that starts with `~/` gets the home folder of the `Env` (`HOME`) in its place. The default name is `strings.TrimSuffix` of the base name, for `.ovpn` in any case.
   - `BeginEdit`: the file is `<a temporary folder of the app, made on first use with dirs.NewTempDir and removed by Close>/<id>.ovpn`, written with `storage.WriteFileAtomic`. `EndEdit`: `storage.ReadFile` (1 MiB), compare with `store.Text`, `store.ReplaceText` when different; the file is removed whatever happened; a path that is not the one `BeginEdit` gave for that id is `AppEditUnknown`.
   - `Connect` (step 3.1) also tells the subscribers when it marked the profile used or saved the user name.
3. `interface.go`:
   ```go
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
       Connect(ctx context.Context, id string, creds *Credentials, remember bool) error
   }
   var _ Application = (*App)(nil)
   ```
   And two helpers that the TUI needs and may not get from the core (SPEC §4.2 rule 2): `func IsRunsScripts(err error) (options string, ok bool)` and `func IsAuthFailed(err error) bool`.
4. `apptest/apptest.go`:
   ```go
   // Fake is an app.Application with fixed content, for the tests of the TUI.
   type Fake struct {
       Snap  app.Snapshot      // what Snapshot returns
       Fail  map[string]error  // the method's name -> the error it returns
       Block map[string]chan struct{} // the method's name -> it waits for this channel (or its ctx)
   }
   func Sample() *Fake // five profiles, the target "firefox", a fixed time
   func Empty() *Fake  // no profile, no target
   func (f *Fake) Calls() []string // "RenameProfile(id-work, Office)", in order; a password is written as "***"
   func (f *Fake) Emit()           // calls the subscribers
   ```
   `Sample()` has, at the fixed time 2026-11-25 12:00 UTC: `Home` (one remote, used an hour ago), `Work` (two remotes, needs a password, user name `ada`, DNS `10.0.0.53`, used three days ago, two ignored options, three files), `work-backup` (never used), a broken profile (its name is its id, the reason `its profile cannot be read: ...`), and one whose name is 64 characters long with a wide character in it. A method with no entry in `Fail` succeeds and changes nothing (the tests set `Snap` themselves and call `Emit`). The fake is safe for several goroutines.
5. `docs/SPEC.md` §12.2 says that the commands "run in a goroutine of the App and answer with an event". This step makes them blocking calls that the TUI runs inside a `tea.Cmd`, with one event (a change of the profiles). Rewrite the paragraph to say that, on this branch.

**Tests**

- `TestSnapshot`: a store with two profiles and one broken: the fields; the order; the target; a profile name with an escape sequence in the store's file is safe in the snapshot.
- `TestCommands`: each command with a real store: add (the default name from `Work.OVPN`; `~/x.ovpn` with the `Env`'s home; a path that does not exist), rename, user name, DNS (`"1.1.1.1, 9.9.9.9"`, empty, `"x"`), delete; after each that worked the subscriber was called once, after each that failed not at all.
- `TestAddWithScripts`: the first call fails and `IsRunsScripts(err)` gives `"up, down"`; the second, with `withoutScripts`, adds.
- `TestEdit`: `BeginEdit` gives a file of mode 0600 with the profile's text; after writing a new valid text into it, `EndEdit` returns `true`, the store has the new text and the file is gone; unchanged text returns `false`; an invalid text returns the parser's error, the store keeps the old text and the file is gone; a path that was not given is `AppEditUnknown`.
- `TestSubscribe`: two subscribers; one cancelled; `Close` after a `BeginEdit` removes the temporary folder.
- `apptest`: `Sample()` and `Empty()`; `Calls` records a call and hides the password of `Connect`; `Fail`; `Block` (the call returns when the channel is closed, and with `ctx.Err()` when the context is cancelled first).

**Finish**: rules 10 to 15.

### Step 5.2: The frame, the list and the profiles page

**Goal**: `tube` and `tube <application>` open the TUI: the frame, the list of profiles, the help page, quit. A script captures its screen as a proof.

**Read**: SPEC §13.1, §13.2, §13.7, §16.4, §16.5. The two lists above.

**Files**: `internal/tui/model.go`, `frame.go`, `keys.go`, `styles.go`, `run.go`, `text.go`, `list.go`, `page_profiles.go`, `page_help.go`; tests `harness_test.go`, `frame_test.go`, `list_test.go`, `text_test.go`, `page_profiles_test.go`, `run_test.go`; `cmd/tube/main.go` and its test; `scripts/tui-proof.sh`; `scripts/testdata/three-profiles/` (the fixture of the proofs).

**Tasks**

1. `go get charm.land/bubbletea/v2@v2.1.2 charm.land/lipgloss/v2@v2.1.1`, then `go mod tidy`. `internal/archtest` already allows them.
2. `text.go`: `fit(s string, width int) string` cuts `s` to `width` cells with `…` as the last cell when it was longer, and fills with spaces to exactly `width` cells. `clean(s string) string` makes pasted or typed text safe for one line: every control character (C0, DEL, C1), new line and tab becomes a space; invalid UTF-8 becomes U+FFFD. `ago(now, then time.Time) string`: `never` for the zero time, `just now` (under a minute), `5 min ago`, `3 hours ago`, `2 days ago`, and the date `2026-11-02` after 30 days.
3. `list.go`: `list` keeps a number of rows, the selected row and the first visible row. Methods: `setLen(n)` (keeps the selection inside), `key(name string) bool` for `up`/`k`, `down`/`j`, `pgup`, `pgdown`, `home`, `end` (true when it used the key), `selected() int`, `window(height int) (first, last int)` (the rows to draw, moved so that the selected one is visible), `selectIndex(i int)`.
4. `keys.go`: the pages (`pageProfiles`, `pageHelp`; later steps add theirs) and **one** table of key bindings: for each page a list of `{keys, what}` such as `{"a", "add"}`. The footer and the help page are both drawn from this table, so they cannot disagree.
5. `styles.go`: the Lip Gloss styles in one struct (title, selected row, dim text, error). Use bold and reverse for "selected"; use color only as an addition.
6. `model.go`: `Model` and `NewModel(ctx context.Context, a app.Application, clk clock.Clock) Model`.
   - `Init` returns the command that reads the snapshot (`snapshotMsg{snap, err}`).
   - `Update` handles: `tea.WindowSizeMsg`; `changedMsg` (returns the command that reads the snapshot again); `snapshotMsg` (keeps it, sets the list's length; the first time, selects the profile with the latest `LastUsed`; later, keeps the selection on the same profile id when it still exists); `tickMsg` (keeps the time, for `ago`); `doneMsg` (the message line becomes `Error: ` + `app.Explain(err)`, or is cleared); `tea.KeyPressMsg`.
   - Any key press clears the message line first. `ctrl+c` and, on the profiles page, `q` return `tea.Quit`. `?` opens the help page; any key closes it.
   - The result: `func (m Model) Result() Result` with `type Result struct{ Run bool }` (step 5.5 sets `Run`).
7. `page_profiles.go` (SPEC §13.2): the rows are `> Name   server   password   last used`: the name takes what is left (at least 12 cells), the server is the first remote and ` +N` when there are more (at most 34 cells), `password` or empty (8 cells), `ago` (12 cells), each through `fit`. A broken profile's row is its name and `broken: <reason>`. The empty list shows `No profiles yet. Press a to add an .ovpn file.` The keys of this step: the list's keys; `enter`, `a`, `e`, `d` show `not built yet` on the message line (the later steps replace them).
8. `frame.go`: `View` draws the screen described above. `page_help.go`: every binding of the table, page by page, then the three lines of SPEC §13.7: what Tube does, that the application has no network without the VPN, and the sentence about programs that hand their work over (§8.7).
9. `run.go`:
   ```go
   type Options struct { In io.Reader; Out io.Writer; Env []string; Clock clock.Clock } // nil fields: the terminal, the process's environment, the system clock
   func Run(ctx context.Context, a app.Application, opts Options) (Result, error)
   ```
   - Make the program with `tea.WithContext(ctx)`, `tea.WithInput`, `tea.WithOutput`, `tea.WithEnvironment` (only for the fields that are set).
   - Subscribe to the app: the listener does `go program.Send(changedMsg{})` (`Send` waits when the program is not running yet, and the app's goroutine must not wait). Cancel the subscription when `Run` returns.
   - Start a goroutine with `clock.NewTicker(time.Second)` that sends `tickMsg{}`; stop it when `Run` returns.
   - An error of `program.Run()` is `errors.NewErr("TuiRunFailed", err)`; a cancelled `ctx` is not an error. The result is the final model's `Result()`.
10. `cmd/tube/main.go`: task 4.5 of step 3.2 becomes: when standard input or standard output is not a terminal (`unix.IoctlGetTermios` on the descriptor fails), the error `TubeNotATerminal` ("the TUI needs a terminal; use --profile <name>"), 125. Else `tui.Run`; with no target, or a result without `Run`, the status is 0 (130 when there was a target and the user quit); with `Run`, go on with `App.Run` as `--profile` does (step 5.5 makes that reachable). Because `run` now needs the two descriptors, its signature takes `stdin *os.File` and `stdout *os.File`; update `main` and the tests.
11. The proof fixture `scripts/testdata/three-profiles/`: a configuration folder with `profiles/<id>/meta.json` and `files/<name>.ovpn` for `Home`, `Work` (with `auth-user-pass`) and `Lab`. Make it with a small Go program, `scripts/testdata/make-three-profiles.go` (`//go:build ignore`), which uses `profilestest` and the store, and commit what it wrote, with a `README` that says: these keys were generated for the proofs, are used nowhere else, and protect nothing.
12. `scripts/tui-proof.sh` (method U), executable. Usage:
    ```
    scripts/tui-proof.sh <out.txt> [--config <fixture dir>] [--size 100x30] [--wait-first <regex>] [--keys '<tmux keys>' | --text '<text>' | --wait '<regex>']... -- [tube arguments]
    ```
    What it does, in this order:
    1. Checks that `tmux`, `go` and `timeout` are installed (else it says which is missing and how to install it, and exits 1).
    2. Builds `tube` into a temporary folder (`go build -o`).
    3. Starts a private tmux server, with `TMUX_TMPDIR` set to the script's temporary folder, `TMUX` unset and `-f /dev/null`, so that the user's own tmux and its configuration are never touched: `tmux -L tube-proof-$$ -f /dev/null new-session -d -s proof -x <cols> -y <rows> "<command>"`, where the command is `exec timeout -k 2 120 scripts/isolated.sh [--config <fixture>] -- <tube> <tube arguments>`, built with `printf '%q '`. Right after, `tmux -L … set-option -t proof remain-on-exit on`, so that the screen can still be read after the program has ended (step 5.5 captures what the application printed after the TUI).
    4. Polls `tmux -L … capture-pane -p -t proof` until a line matches `--wait-first` (`Tube` by default; 15 seconds at most, else it fails and prints the screen).
    5. Goes through the `--keys`, `--text` and `--wait` arguments in the order they were given: `--keys` sends tmux key names (`a`, `Enter`, `Escape`, `C-c`, separated by spaces) with `tmux send-keys` and waits 0.3 seconds; `--text` types its value as it is (`send-keys -l`); `--wait` polls until a line of the screen matches the regular expression (`grep -E`; 15 seconds at most, else it fails and prints the screen).
    6. Waits 0.5 seconds, and saves `capture-pane -p` to `<out.txt>`.
    7. Always, also after a failure (a `trap`): `tmux -L … kill-server`, removes the temporary folder.
    Exit status 64 for a mistake in its own arguments.

**Tests**

- `text_test.go`: `fit`: shorter, equal, longer; wide characters (`"日本語"` in 5 cells is `"日本…"`); width 0 and 1. `clean`: an escape sequence, a new line, a tab, invalid UTF-8. `ago`, a table.
- `list_test.go`: every key at the top, the middle and the bottom; `setLen` smaller than the selection; `window` keeps the selection visible; an empty list.
- `frame_test.go` and `page_profiles_test.go`, with `apptest.Sample()` and `apptest.Empty()`:
  - `TestGoldenProfiles`, a table: `{sample, 100×30}`, `{sample, 60×16}`, `{sample, 40×10}`, `{empty, 100×30}`, `{sample without a target, 100×30}`. Goldens `profiles-<name>.txt`. In the sample at 100×30 the selected row is `Home` (the latest used).
  - `TestGoldenHelp`: `?` at 100×30; a second key closes it.
  - `TestTooSmall`: 39×10 and 40×9 show only `The terminal is too small`; 40×10 shows the frame.
  - `TestMoving`: `down`, `j`, `end`, `home`, `pgdown` change the selected profile (assert the id).
  - `TestALongList`: a snapshot of 100 profiles at 100×20: the selected row is always on the screen while moving to the end and back.
  - `TestEveryLineFits`: for widths 40, 60, 100 and 137 and heights 10, 30 and 50: the view has exactly `H` lines and no line is wider than `W` (`lipgloss.Width`).
  - `TestChangedReadsTheSnapshotAgain`: change the fake's `Snap`, send `changedMsg{}`: the list follows, and the selection stays on the same profile id.
  - `TestASnapshotThatFails`: `Fail["Snapshot"]`: the message line shows the error.
  - `TestQuitKeys`: `q` and `ctrl+c` set `h.quit`; `Result().Run` is false.
  - `TestHostileNames`: a profile name that was made safe (it has U+FFFD) is drawn, and the screen has no byte below 0x20 but the new line.
- `run_test.go`:
  - `TestRunWritesOnlyToItsWriter`: `Run` with `In: strings.NewReader("q")`, `Out: &buffer`, the sample fake, under a `ctx` of 10 seconds: it returns, the buffer is not empty.
  - `TestRunEndsWhenTheContextIsCancelled`: an input that never ends (`io.Pipe`), cancel the context: `Run` returns within 5 seconds with no error.
  - `TestRunSendsTheAppsChanges`: `In` is an `io.Pipe`; after the fake's `Snap` changed and `Emit()`, the output eventually has the new profile's name; then write `q`.
- `cmd/tube`: `run` with standard input that is a pipe and no `--profile`: 125 and the sentence of `TubeNotATerminal`.

**Proof** (U):
```
scripts/tui-proof.sh docs/implemented-proof-images/5.2-profiles.txt --config scripts/testdata/three-profiles -- true
scripts/tui-proof.sh docs/implemented-proof-images/5.2-help.txt --config scripts/testdata/three-profiles --keys '?' -- true
```
(`true` is the application: it is never started in this step.) `5.2-profiles.txt` must show the header `select a profile to run true`, the three profiles with `Work` marked `password`, and the footer; `5.2-help.txt` the list of keys. Afterwards `pgrep -a tmux` and `pgrep -a tube` show nothing of yours.

**Finish**: rules 10 to 15.

### Step 5.3: Adding and deleting

**Goal**: `a` adds a profile from a path, with the completion of file names and the question about scripts; `d` deletes after a question.

**Read**: SPEC §13.3, §6.2, §13.2 (`d`).

**Files**: `internal/tui/field.go`, `question.go`, `complete.go`, `page_add.go`, `model.go`, `keys.go`, `page_profiles.go`, with tests.

**Tasks**

1. `field.go`: `field`, a one-line text input: the text as runes and a cursor. Keys: a typed character (`msg.Text`) is inserted (after `clean`); `left`, `right`, `home`/`ctrl+a`, `end`/`ctrl+e`, `backspace`, `delete`, `ctrl+u` (empty it), `ctrl+w` (delete the word before the cursor). `paste(s)` inserts `clean(s)`. `limit` is the largest number of runes (more is not taken). `secret bool`: `View` then draws nothing of the text and no cursor that moves (the field looks empty; step 5.5 uses it). `View(width)` shows the part of the text around the cursor and draws the cursor with the reverse style; an empty field shows its placeholder in the dim style. `set(s)` replaces the text and puts the cursor at the end.
2. `question.go`: `question{text string, yes string}`, drawn as a box in the middle of the page's part: the text (wrapped to the box), then `[y] <yes>   [n] Cancel`. `y` or `Y` answers yes; `n`, `N`, `esc` and `enter` answer no (the default is **no**: Enter never destroys anything). Any other key does nothing. The model gets one place for an open question: a field `ask *asking` with the question and the command to run on yes. While it is open, it gets every key.
3. `complete.go`: a pure function, so that it is tested without a disk:
   ```go
   // complete finishes the last part of a path as far as the names in its folder allow.
   // input is what the user typed; home replaces a leading "~". It returns the new input
   // and the names that match when there is more than one (at most 20).
   func complete(input, home string, readDir func(dir string) ([]os.DirEntry, error)) (string, []string)
   ```
   One match: the name is finished, with a `/` after a folder. Several: the longest common beginning is filled in, and the names are returned (folders with a `/`). None, or a folder that cannot be read: the input is unchanged. Names that start with `.` match only when the typed part starts with `.`. Every name goes through `clean` before it is shown.
4. `page_add.go` (SPEC §13.3): two fields, `Path of the .ovpn file` and `Name` (placeholder: `the file's name`); `tab` in the path field completes (the candidates are shown under the fields, in columns, until the next key); `down`/`up` or `tab` in the name field move between the fields; `enter` returns the command that calls `AddProfile(ctx, path, name, false)`; `esc` goes back to the list. The result:
   - success: back to the list, the message `Added <name>.`, the new profile selected (the command's message carries the id);
   - `app.IsRunsScripts(err)`: the question `This profile runs scripts (<options>). Tube never runs them, and sets the DNS of the application itself. Add it without them?` with `yes` = `Add without them`; yes calls `AddProfile(..., true)`;
   - another error: stays on the page, and the sentence of `app.Explain(err)` is drawn under the fields (wrapped), until the next key.
   While the command runs, the page shows `Adding…` and takes no key but `ctrl+c`.
5. `page_profiles.go`: `a` opens the add page; `d` on a profile opens the question `Delete the profile "<name>"? Its copy of the .ovpn file is removed; the file you added it from is not touched.` with `yes` = `Delete`; yes calls `DeleteProfile` and shows `Deleted <name>.` Both keys get their rows in `keys.go`.

**Tests**

- `field_test.go`: typing, every editing key, the limit, a paste with control characters (they become spaces), the view when the text is longer than the width (the cursor is visible), `secret` (the view has nothing of the text, and is the same for one and for ten characters).
- `question_test.go`: `y` is yes; `n`, `esc`, `enter` are no; another key is nothing; a golden.
- `complete_test.go`, a table with a fake `readDir`: one file; one folder (the `/`); several with a common beginning; several with none; no match; `~/`; a hidden file; a folder that cannot be read; an empty input (the names of the working folder).
- `page_add_test.go`: `TestGoldenAdd` (the empty page; with text and candidates; with an error of the parser, made with a real `app.App` and a broken profile file, so that the sentence is the real one: `The option "comp-lzo" on line 3 …`); `TestAddFlow` with the fake: typing a path, `enter`: `Calls()` has `AddProfile(/tmp/x.ovpn, , false)`; `TestAddWithScriptsFlow`: `Fail["AddProfile"]` is a `ProfileRunsScripts` error the first time: the question; `y`: the second call has `true`; `n`: no second call; `TestAddReal` with a real app: a good file is added and selected; `TestEscGoesBack`.
- `page_profiles_test.go`: `TestGoldenDeleteQuestion`; `TestDeleteFlow`: `d`, `y`: `Calls()` has `DeleteProfile(<id>)`; `d`, `enter`: no call; `d` on an empty list does nothing.
- `TestEveryLineFits` of step 5.2 gets the add page and the question.

**Proof** (U): make a broken profile file in a temporary folder first (`printf 'client\nremote 127.0.0.1 1194\ncomp-lzo\n' > "$dir/bad.ovpn"`), then:
```
scripts/tui-proof.sh docs/implemented-proof-images/5.3-add.txt --config scripts/testdata/three-profiles --keys 'a' --text "$dir/bad.ovpn" --keys 'Enter' --wait 'comp-lzo' -- true
scripts/tui-proof.sh docs/implemented-proof-images/5.3-delete.txt --config scripts/testdata/three-profiles --keys 'd' --wait 'Delete the profile' -- true
```
`5.3-add.txt` must show the add page with the sentence about `comp-lzo`; `5.3-delete.txt` the question.

**Finish**: rules 10 to 15.

### Step 5.4: The profile's page and the editor

**Goal**: `e` opens a profile's page: everything about it, and the keys that rename it, set its user name and DNS servers, and edit its text in the user's editor.

**Read**: SPEC §13.4, §13.5, §6.3.

**Files**: `internal/tui/page_profile.go`, `editor.go`, `model.go`, `keys.go`, with tests.

**Tasks**

1. `page_profile.go` (SPEC §13.4): the page draws, each on its own lines and cut to the width: `Name`, `Servers` (every remote, at most 8 lines, then `… and N more`), `Login` (`a user name and a password` or `a certificate only`), `User name` (or `not remembered`), `DNS servers` (or `from the VPN`), `Last used`, `Files` (or `inside the profile`), `Ignored options` (wrapped, at most 3 lines). A broken profile shows its reason and takes only `d` and `esc`.
2. One field at a time, opened by a key and drawn under the lines: `r` (rename, filled with the name, limit 64), `u` (user name, limit 128), `n` (DNS servers, filled with the current ones joined by `, `, limit 200, placeholder `empty: from the VPN`). `enter` returns the command (`RenameProfile`, `SetUsername`, `SetDNS`); `esc` closes the field. An error is shown on the message line and the field stays open with its text.
3. `editor.go`:
   ```go
   // findEditor chooses the editor: $VISUAL, else $EDITOR, else the first of nano and vi that
   // lookPath finds. The value of a variable is split at spaces into the program and its
   // arguments; it is never given to a shell. ok is false when there is none.
   func findEditor(getenv func(string) string, lookPath func(string) (string, error)) (argv []string, ok bool)
   ```
   A variable whose program `lookPath` does not find is skipped.
4. `t` on the page: the command calls `BeginEdit`; its message (`editBeganMsg{id, path, err}`) makes the model return the command of the field `execEditor`, which by default is `tea.ExecProcess(exec.Command(argv[0], append(argv[1:], path)...), func(err error) tea.Msg { return editorDoneMsg{id, path, err} })`; `editorDoneMsg` returns the command that calls `EndEdit`, whose message shows `Saved.`, `Nothing changed.`, or the error (`The profile was not changed: <app.Explain(err)>`). With no editor: the message `No editor found. Set EDITOR.` and no call of `BeginEdit`. When the editor itself failed to start, `EndEdit` is still called (it removes the file).
5. `d` deletes as on the list and goes back to it; `esc` goes back; `enter` is step 5.5's. `e` on the list opens this page; the rows in `keys.go`.

**Tests**

- `TestGoldenProfilePage`, a table at 100×30 and 60×16: `Work` of the sample (everything set), `work-backup` (nothing set), the broken one, a profile with 12 remotes.
- `TestRename`, `TestUsername`, `TestDNS`: the key, typing, `enter`: `Calls()` has the call with the typed text; `esc` makes no call; a failing call keeps the field open and shows the sentence.
- `TestFindEditor`, a table: `VISUAL=code -w` (found) gives `[code -w]`; `VISUAL` not found falls to `EDITOR`; neither set gives `nano`; `nano` not found gives `vi`; nothing found gives `ok == false`; an empty variable is not set.
- `TestEditFlow`, with the fake and a replaced `execEditor` that returns `editorDoneMsg` at once: `Calls()` is `BeginEdit(id)`, then `EndEdit(id, path)`; the message is `Saved.` when the fake returns `true`. `TestEditWithoutAnEditor`: no call at all.
- `TestEditReal`, with a real `app.App` and an `execEditor` that runs a program of the test instead of giving the terminal away (a shell script that appends a comment line to the file it is given; run it with `exec.Command(...).Run()` inside the returned command): the store's text has the new line. With a script that writes garbage: the message starts with `The profile was not changed:` and the store's text is the old one.
- `TestEveryLineFits` gets this page.

**Proof** (U):
```
scripts/tui-proof.sh docs/implemented-proof-images/5.4-profile.txt --config scripts/testdata/three-profiles --keys 'e' --wait 'Servers' -- true
```
It must show the page of the selected profile with every label of task 1.

**Finish**: rules 10 to 15.

### Step 5.5: Login, connecting and running

**Goal**: `enter` on a profile connects it and runs the application: the login page for a profile with a password, the connecting page, then the TUI ends and the application starts on the user's terminal. This is the step that makes `tube firefox` work. (SPEC §13.6 and §13.8 describe the end state, in which the TUI stays open with tabs: that is M6. Until step 6.2, the TUI ends after connecting, as this step says. Write this step as it is described here: the pages, their tests and the flow up to "connected" stay.)

**Read**: SPEC §13.6, §10, §12.2. The line about the password in the box of M5.

**Files**: `internal/tui/page_login.go`, `page_connecting.go`, `model.go`, `keys.go`, `page_profiles.go`, `page_profile.go`, with tests; `cmd/tube/main.go`; `cmd/tube/tui_server_test.go` (tag `tube_server`); `internal/core/vpn/vpntest/cmd/server/main.go` (tag `tube_server`); `docs/MANUAL-CHECKS.md`.

**Tasks**

1. `enter` on a usable profile, on the list or on its page, when the snapshot has a target: a profile with `NeedsPassword` opens the login page; another goes to connecting at once. Without a target, `enter` on the list opens the profile's page (SPEC §13.2). A broken profile shows `This profile cannot be used: <reason>`.
2. `page_login.go`: the fields `User name` (filled with the remembered one) and `Password` (`secret`), and the line `[x] remember the user name` (on by default; `space` on that line turns it; `tab`, `down`, `up` move between the three). The cursor starts in the password field when a user name is filled in. `enter` with an empty user name or an empty password shows `Give the user name and the password.`; else it goes to connecting. `esc` goes back and clears the password.
3. `page_connecting.go`: the page shows `Connecting to <name>…  <N> s` (the seconds from the `tickMsg` times) and `esc cancels`. Entering it: make a context with a cancel function from the model's `ctx`, keep the cancel function in the model, and return the command that calls `Connect(connectCtx, id, creds, remember)` and returns `connectedMsg{err}`.
   - `esc`: call the cancel function; show `Cancelling…`; when `connectedMsg` arrives, go back to the list (or the login page when there was one) with the message `Cancelled.`
   - `connectedMsg` with an error: `app.IsAuthFailed(err)` goes back to the login page with the user name kept, the password cleared, and the sentence on the message line; any other error goes back to the list with the sentence.
   - `connectedMsg` without an error: clear the password, set the result's `Run` to true, and return `tea.Quit`.
   - `ctrl+c` on this page cancels and quits.
4. `cmd/tube/main.go`: nothing new but what step 5.2 prepared: after `tui.Run` returned a result with `Run`, the same code as `--profile` runs the application (`App.Run` with the signals). Check that the terminal is back to normal before the application starts: `tui.Run` has returned, so Bubble Tea has left the alternate screen.
5. `vpntest/cmd/server/main.go` (tag `tube_server`, `package main`): starts the test server outside a test, for the proofs: it makes the keys and the configuration in a folder given by `--dir`, starts `openvpn`, writes the client profile to `<dir>/test.ovpn`, prints `ready <dir>/test.ovpn <the server's tunnel address>`, and waits for SIGTERM, on which it stops the server. `vpntest.StartServer` takes a `testing.TB`: give this program a small type that implements the methods of `testing.TB` it uses (`Helper`, `Fatalf`, `Skip`, `TempDir`, `Cleanup`, `Logf`), or split `StartServer` into a function that takes no `testing.TB` and a wrapper; choose the one that changes less, and say which in the entry.
6. `docs/MANUAL-CHECKS.md`, three items for this step: (a) `tube firefox -P <a new Firefox profile>` with a real VPN profile: a "what is my IP" page shows the VPN's address, and a normal Firefox beside it shows the machine's; (b) with a normal Firefox already open, `tube firefox` does not open a window in it (it refuses, or says that Firefox is already running); (c) a real provider's `.ovpn` file can be added in the TUI, and the errors for a file that Tube cannot use are understandable.

**Tests**

- `TestGoldenLogin` (with the remembered user name; empty), `TestGoldenConnecting` (at 0 and at 12 seconds: send `tickMsg` with times of the fake clock).
- `TestThePasswordIsNeverDrawn`: type `hunter2` into the password field: `h.screen()` never has `hunter2`, and the screen with 3 characters typed is the same as with 7.
- `TestConnectFlow`, with the fake: `enter` on `Home` (no password): `Calls()` is `Connect(id-home, nil, false)`; the model quit and `Result().Run` is true.
- `TestLoginFlow`: `enter` on `Work`; type the password; `enter`: `Calls()` is `Connect(id-work, ada/***, true)`; with the remember line turned off: `false`.
- `TestAWrongPassword`: `Fail["Connect"]` is an error for which `app.IsAuthFailed` is true: the login page is shown again, the user name is `ada`, the password field is empty, the message line has the sentence.
- `TestAConnectError`: another error: the list, with the sentence.
- `TestCancel`: `Block["Connect"]` holds the call; `esc`: the fake's context was cancelled (the call returned with `context.Canceled`), the page is the list, the message is `Cancelled.`
- `TestEnterWithoutATarget`: opens the profile's page; no `Connect`.
- `TestABrokenProfile`: `enter` shows the reason; no call.
- `TestEveryLineFits` gets both pages.
- `cmd/tube/tui_server_test.go` (tag): `TestTheTUIRunsPing`: the real binary in a pseudo-terminal that the test makes (`pty.Open()` of step 2.5; the slave side is the binary's standard input, output and error, with `SysProcAttr{Setsid: true, Setctty: true}`; set the size with `pty.SetSize`); read the master side until `test` (the profile's name) is on the screen; write `\r`; read until `bytes from 10.8.0.1`; the process ends with status 0. A helper reads the master side in a goroutine into a buffer that the test polls.

**Proof** (U), inside the namespace with a real server. First give `scripts/tui-proof.sh` the option `--config-in-place`: with it, the folder of `--config` is used as it is and not copied (the script runs the program as `scripts/isolated.sh -- env TUBE_CONFIG_DIR=<the folder> <tube> ...`), so that a profile added in one call is there in the next. Then:
```sh
scripts/vpn-ns.sh -- sh -c '
  set -e
  dir=$(mktemp -d); mkdir -m 700 "$dir/cfg"
  go build -tags tube_server -o "$dir/server" ./internal/core/vpn/vpntest/cmd/server
  "$dir/server" --dir "$dir" > "$dir/out" &
  server=$!
  trap "kill -TERM $server; wait $server; rm -rf $dir" EXIT
  until grep -q ready "$dir/out"; do sleep 0.2; done
  # add the server'"'"'s profile through the TUI itself
  scripts/tui-proof.sh "$dir/added.txt" --config-in-place --config "$dir/cfg" --keys a --text "$dir/test.ovpn" --keys Enter --wait "Added" -- true
  # select it: the TUI ends and ping runs
  scripts/tui-proof.sh docs/implemented-proof-images/5.5-ran.txt --config-in-place --config "$dir/cfg" --wait-first test --keys Enter --wait "packets transmitted" -- ping -c 2 -W 2 10.8.0.1
'
```
`5.5-ran.txt` must show the output of `ping` with `2 packets transmitted, 2 received`. For `5.5-connecting.txt`, outside the namespace: a profile whose remote is a UDP port of `127.0.0.1` where nothing listens (write it with `printf` into a temporary folder: `client`, `dev tun`, `proto udp`, `remote 127.0.0.1 9`, and a `<ca>` block with any certificate that `profilestest` makes; or add a small `//go:build ignore` program next to the fixture's that writes it), added the same way, then `--keys Enter --wait 'Connecting to'`. It must show the connecting page with the profile's name and the seconds. Afterwards `pgrep -a openvpn`, `pgrep -a tmux` and `pgrep -a tube` show nothing of yours.

**Finish**: rules 10 to 15. **Last step of M5**: `devVersion` becomes `0.7.0-dev`; tag `v0.6.0` with the message `M5: The TUI: the profiles`; push.

---

---

## M6: The TUI: the running application (v0.7.0)

**Read this before any step of M6.** The box of M5 holds for M6 too (the facts about the libraries, and how the TUI is built). What M6 adds:

- **The TUI stays open while the application runs** (the user, 2026-11-25). After the connecting page, the model shows the *run screen*: tabs, and under them the tab's page.
- **The screen** of the run screen (width `W`, height `H`):
  ```
  line 0        Tube   firefox through Work   connected 00:12:31   ↓ 1.2 MB/s  ↑ 80 kB/s
  line 1        [Status]  Output  Network
  line 2        ────────────────────────────────────────────── (W cells)
  line 3…H-4    the tab's page (H-6 lines)
  line H-3      ────────────────────────────────────────────── (W cells)
  line H-2      the message line
  line H-1      the keys of the tab
  ```
  The current tab is in brackets and bold. The state's words on line 0: `connected <time since>`, `connecting again…`, `disconnected`, `login refused`; after the application ended, line 0 says `<application> ended (status N)` in place of the application's name.
- **The application's output comes through a pseudo-terminal.** Tube holds the master side; the application has the other side as its terminal, with `TERM=dumb`. `termlog` turns what it writes into safe lines. Keys typed on the Output tab are written to the master side, so the application reads them as if they were typed on its terminal.
- **The model holds everything it draws.** On each `changedMsg` and each `tickMsg` it returns one command that reads `RunState()` and the lines of the output that are on the screen (`OutputLen`, `OutputRange`), and the message with both is kept. `View` never calls the app.
- **Two ways of running** (SPEC §11): with the TUI the application runs on the pseudo-terminal and the TUI stays; with `--profile` the application gets the user's terminal and there is no TUI. `cmd/tube` chooses.

### Step 6.1: Running the application under the App

**Goal**: the App can start the application on a pseudo-terminal without blocking, say what the session and the application are doing, give the output's lines, take input, disconnect, switch to another profile, and stop.

**Read**: SPEC §12.4, §10, §13.8 to §13.10.

**Files**: `internal/app/run.go`, `runstate.go`, `interface.go`, with tests; `internal/app/run_server_test.go` (tag `tube_server`); `internal/app/apptest/apptest.go`; `internal/core/vpn/vpntest/server.go` (the option `Subnet`).

**Tasks**

1. `runstate.go`:
   ```go
   // Traffic is what passed in one direction.
   type Traffic struct {
       Bytes, Packets uint64
       Rate           float64 // bytes a second, over the last second
   }
   // Event is a line that Tube had to say while the application runs.
   type Event struct {
       Time time.Time
       Text string // without the "tube: " in front; safe to draw
   }
   // RunState is what the run screen draws from.
   type RunState struct {
       Started     bool      // Start was called and worked
       AppName     string    // the application's name (the base name of argv[0]), safe to draw
       AppRunning  bool
       Exit        int       // the exit status, once it has ended
       State       string    // "connected", "reconnecting", "disconnected", "refused"
       Since       time.Time // when the state began
       ProfileID   string    // the profile in use, or the last one
       ProfileName string
       Addrs       []string  // the tunnel's addresses, "10.8.0.2/24"
       DNS         []string  // the DNS servers in use
       MTU         int
       Down, Up    Traffic   // Down is from the VPN to the application
       Dropped     uint64
       Events      []Event   // the last 100, oldest first
       Now         time.Time
   }
   ```
2. `run.go`:
   ```go
   // Start starts the application on a pseudo-terminal, through the VPN that Connect made.
   // It returns when the application has started; it does not wait for its end.
   func (a *App) Start(ctx context.Context) error
   func (a *App) RunState() RunState
   // OutputLen and OutputRange are termlog's, for the application's output.
   func (a *App) OutputLen() (first, end int)
   func (a *App) OutputRange(from, to int) []string
   // Input writes what the user typed to the application's terminal.
   func (a *App) Input(p []byte) error
   // ResizeOutput tells the application's terminal its size.
   func (a *App) ResizeOutput(cols, rows int)
   // Disconnect takes the network away from the application.
   func (a *App) Disconnect()
   // Switch connects the profile and gives it to the running application in place of the VPN
   // it has. The VPN in use stays until the new one is connected; when the new one fails,
   // nothing changes.
   func (a *App) Switch(ctx context.Context, id string, creds *Credentials, remember bool) error
   // Stop asks the application to end (SIGTERM, and SIGKILL five seconds later).
   func (a *App) Stop()
   // Finish stops the application when it still runs, waits for it, cleans up, and returns
   // its exit status. cmd/tube calls it after the TUI has ended.
   func (a *App) Finish() (int, error)
   ```
   - `Start`: with no tunnel `AppNotConnected`, with no target `AppNoTarget`, a second call `AppAlreadyStarted`. `sandbox.LookPath`; `pty.Open()`; a `termlog.New(5000, 1000)`; `session.Start` with the slave side as the three streams and `Terminal: true` (both in `Options.Sandbox`), `ExtraEnv: []string{"TERM=dumb"}`, `Lines` set to a writer that turns each line into an `Event` (the `tube: ` in front removed, through `sanitize.Display(s, 300)`, with the clock's time; the last 100 are kept), `OnChange` set to the App's notifying, and the `Redial` of step 4.1. After the session started, close the App's copy of the slave side, and start one goroutine that copies the master side into the log until reading fails (an `EIO` is the normal end: every program that had the terminal is gone).
   - A sampler goroutine, on the App's clock (a ticker of one second): keeps the traffic of the last two samples, so that `RunState` can give the rates. It stops when the application ends.
   - `RunState` is made from `session.Status()` and the sampler. Before `Start`, it is the zero value with `Now`.
   - Notifying: the log's `OnWrite`, the session's `OnChange`, a new event and the sampler all call the App's subscribers (step 5.1). So that a program that prints fast does not flood the TUI, the calls are joined: a channel of size 1 and one goroutine that calls the subscribers and then waits 50 ms of real time before it takes the next.
   - `Input`: writes to the master side (Tag `AppInputFailed`); after the application ended it does nothing. `ResizeOutput`: `pty.SetSize`, ignoring a failure.
   - `Switch`: `store.Get`, `vpn.Dial` (as `Connect` does, with the same logging and the same remembering of the user name), then `session.Use` with the tunnel, the profile's name, its DNS servers and a `Redial` for the new profile and credentials. When the dial fails, the error is returned and the session is not touched. When `Use` fails, its error is returned (the session is then disconnected). The credentials of the profile in use are the ones kept in memory for redialing; the old ones are dropped.
   - `Finish`: `Stop` when the application runs; `session.Wait`; close the master side; wait for the copying goroutine and the sampler. `App.Close` calls it when it was not called.
   - `App.Run` of step 3.1 (the `--profile` way) stays as it is: the application gets Tube's own terminal.
3. `interface.go`: `Application` gets `Start`, `RunState`, `OutputLen`, `OutputRange`, `Input`, `ResizeOutput`, `Disconnect`, `Switch` and `Stop`. `Finish` is not in it: only `cmd/tube` calls it.
4. `apptest/apptest.go`: the fake gets `Run app.RunState` (what `RunState` returns), `Out []string` (the lines of the output, numbered from 0), `Typed() []byte` (everything given to `Input`), `Size() (cols, rows int)` (the last `ResizeOutput`), and its `Calls()` records `Start()`, `Disconnect()`, `Switch(id, user/***, remember)`, `Stop()`. Add `Running() *Fake`: the sample with a started application: `firefox` running through `Work`, connected for 12 minutes and 31 seconds at the fixed time, the address `10.8.0.2/24`, the DNS server `10.0.0.53`, MTU 1500, 1.5 GB down and 80 MB up with rates of 1.2 MB/s and 80 kB/s, 3 packets dropped, four events, and 40 lines of output.

**Tests**

- With a fake tunnel that answers pings (`nstest.Require`, `TestMain` as trap 9, `storagetest.Isolate`, the fake clock; the tunnel is set through the unexported method of step 3.1):
  - `TestStartAndFinish`: the target `sh -c 'echo hello; exit 3'`: after `Start`, `RunState().Started` is true; soon `AppRunning` is false and `Exit` is 3; `OutputRange` has the line `hello`; `Finish` returns 3; afterwards no process and no temporary folder is left.
  - `TestTheOutputIsLive`: the target prints a line, waits for a file, prints another: the first line is in the log before the file is made.
  - `TestTheTerminal`: the target `sh -c 'echo $TERM; tty'`: the output has `dumb` and a `/dev/pts/` path.
  - `TestInput`: the target `sh -c 'read x; echo got:$x'`; `Input([]byte("abc\n"))`: the output has `got:abc`. `TestCtrlC`: the target `sleep 30`; `Input([]byte{3})`: the application ends with status 130.
  - `TestResize`: the target `sh -c 'sleep 0.3; stty size'` (skips without `stty`), `ResizeOutput(100, 30)` before it prints: the output has `30 100`.
  - `TestRunState`: the fields while connected (the addresses, the MTU, the profile's name); after a ping, `Down.Packets` and `Up.Packets` are at least 1; after the fake clock advanced a second with traffic in it, a rate is above 0, and after another second without traffic it is 0.
  - `TestEvents`: the event `connected to work; running sh` is there, without `tube: `, with the fake clock's time; 150 events keep the last 100.
  - `TestDisconnect`: `Disconnect()`: `RunState().State` is `disconnected`, and the event is there.
  - `TestStop`: the target `sleep 30`; `Stop()`: it ends with 143. `TestFinishStopsARunningApplication`.
  - `TestSubscribersAreTold`: a subscriber is called after output, after a change of state, and after the end of the application; a target that prints 10000 lines calls it fewer than 200 times.
  - `TestStartTwice` (`AppAlreadyStarted`), `TestStartWithoutConnect` (`AppNotConnected`).
- `run_server_test.go` (tag): `TestSwitch`: two servers (the second on `10.9.0.0/24`: give `vpntest.ServerOptions` a `Subnet netip.Prefix`); connect the first; the target is the probe `ping-on-demand` (step 4.2); `Switch` to the second profile: a ping to the second server's tunnel address works and one to the first's fails; `RunState()` has the second profile's name and address. `TestSwitchToAWrongPassword`: the switch fails with the sentence of `KindAuthFailed` and the first VPN is still in use (a ping to the first server still works). `TestSwitchAfterDisconnect`.
- `apptest`: `Running()`; the new calls are recorded; `Typed` and `Size`.

**Finish**: rules 10 to 15.

### Step 6.2: The Status and the Output tabs

**Goal**: after connecting, the TUI starts the application and shows the run screen with two tabs: Status (the connection and the traffic) and Output (what the application prints). Quitting stops the application.

**Read**: SPEC §13.8, §13.9, §13.11, §10 ("Tube's own failures"), §11.

**Files**: `internal/tui/run_screen.go`, `tab_status.go`, `tab_output.go`, `units.go`, `model.go`, `keys.go`, `page_connecting.go`, `run.go`, with tests; `cmd/tube/main.go`, `cmd/tube/tui_server_test.go`.

**Tasks**

1. `page_connecting.go`: a `connectedMsg` without an error no longer quits. It returns the command that calls `Start(ctx)` and returns `startedMsg{err}`. With an error: back to the list with the sentence (and `Disconnect` is not needed: `App.Close` closes a tunnel that was never run). Without: the model shows the run screen on the Status tab, and calls `ResizeOutput` with the size of the Output tab's page (through a command).
2. `units.go`, pure functions: `bytesText(n uint64) string` (`0 B`, `999 B`, `1.0 kB`, `1.5 MB`, `2.3 GB`; decimal units, one digit after the point from kB on), `rateText(bytesPerSecond float64) string` (`bytesText` and `/s`), `durationText(d time.Duration) string` (`00:00:05`, `01:02:03`, `2 days 03:04:05`), `countText(n uint64) string` (`1,234,567`).
3. `run_screen.go`: the frame of the box above: line 0 from `RunState` (cut from the right when the terminal is narrow: the rates go first, then the time), the tabs line, the page, the message line, the keys. The tabs are `Status` and `Output` (step 6.3 adds `Network`). Keys on every tab: `tab` goes to the next tab and `shift+tab` to the one before, around the ends. A `tea.WindowSizeMsg` also returns the command that calls `ResizeOutput(W, H-6)`.
4. `tab_status.go`: lines, each cut to the width (labels in a column of 14 cells):
   ```
   Application   firefox, running
   VPN           Work, connected for 00:12:31
   Address       10.8.0.2/24
   DNS servers   10.0.0.53
   MTU           1500

   Received      1.5 GB   1,204,331 packets   1.2 MB/s
   Sent          80.0 MB    611,020 packets   80.0 kB/s
   Dropped       3 packets

   12:00:01  connected to Work; running firefox
   12:10:44  the VPN connection ended (the server did not answer); the application has no network; connecting again…
   12:10:47  connected again
   ```
   The application's line is `<name>, running` or `<name>, ended with status N`. The VPN's line follows the state: `connected for <time>`, `connecting again… (<time>)`, `disconnected`, `the server refused the login`. Without a tunnel, `Address`, `DNS servers` and `MTU` show `-`. The events fill what is left of the page, the newest last; when there are more than fit, the newest are shown. Keys of this tab: `q` and `ctrl+c` quit (task 6); `?` the help page.
5. `tab_output.go`: the lines of the output that fit the page, each through `fit`. It follows the end; `pgup` and `pgdown` move by a page, `home` goes to the oldest kept line, `end` follows again; when it does not follow, the keys line says `scrolled back · end: follow`. An empty output shows `No output yet.` in dim text. **Every other key press goes to the application**, through a command that calls `Input`: a key with text sends `msg.Text`; `enter` sends `\r`; `backspace` sends `0x7f`; `ctrl+c` sends `0x03`; `ctrl+d` sends `0x04`; `ctrl+u` sends `0x15`; `esc` sends `0x1b`; pasted text is sent as it is. After the application ended, they do nothing, and `q` and `ctrl+c` quit. The keys line: `keys go to <application> · tab: next tab · pgup pgdn: scroll`.
   The model keeps `outFirst`, `outEnd` and the lines on the screen; the command of the box reads `OutputLen` and then `OutputRange` for the window (the last `H-6` lines when it follows, else from the line it holds on).
6. Quitting. While the application runs, `q` or `ctrl+c` on the Status tab opens the question `<application> is still running. Stop it and quit?` with `yes` = `Stop and quit`; yes returns the command that calls `Stop()`, the message line says `Stopping <application>…`, and when a `RunState` with `AppRunning` false arrives, the model quits. A second `ctrl+c` while it is stopping quits at once (`Finish` then kills). After the application ended by itself, the message line says `<application> ended with status N. Press q to quit.` and `q` quits with no question.
7. `run.go`: `Result` becomes `struct{ Started bool }`: whether `Start` worked. `cmd/tube/main.go`: after `tui.Run`: with `Started`, the status is `App.Finish()`'s; without it, 0 with no target and 130 with one. The code path of step 5.5 that ran the application after the TUI is removed; `--profile` still uses `App.Run`. SIGTERM and SIGHUP to Tube while the TUI runs cancel its context: `tui.Run` returns, and `Finish` stops the application.
8. The help page gets the keys of the run screen and its tabs, from the table of `keys.go`.

**Tests** (with `apptest.Running()` unless said)

- `units_test.go`: tables for the four functions, with the edges (999 and 1000 bytes, 59 and 60 seconds, 24 hours).
- `TestGoldenStatus`: at 100×30 and 60×16: running and connected; reconnecting; disconnected; refused; the application ended with status 1; more events than fit.
- `TestGoldenOutput`: at 100×30: 40 lines (the last 24 show); scrolled back by a page; an empty output; a line wider than the screen ends in `…`.
- `TestGoldenRunHeader`: at 100, 60 and 40 columns: what is cut first.
- `TestTabs`: `tab` and `shift+tab` go around; the current tab is in brackets.
- `TestTheStartFlow`: with `apptest.Sample()`: `enter` on `Home`: `Calls()` is `Connect(...)`, `Start()`; the run screen shows. With `Fail["Start"]`: the list, with the sentence, and no run screen.
- `TestKeysGoToTheApplication`: on the Output tab, typing `ls`, `enter`, `ctrl+c`: `Typed()` is `ls\r\x03`; `tab` is not sent and changes the tab; `pgup` is not sent and scrolls; on the Status tab nothing is sent.
- `TestPaste`: a `tea.PasteMsg` on the Output tab is sent as it is.
- `TestScrolling`: `pgup` holds the window while new lines arrive (change the fake's `Out`, `Emit()`): the screen does not move; `end` follows again.
- `TestResize`: a `tea.WindowSizeMsg{120, 40}`: `Size()` is `(120, 34)`.
- `TestQuitWhileRunning`: `q`: the question; `n`: nothing; `q`, `y`: `Calls()` has `Stop()`; then set the fake's `Run.AppRunning` to false and `Emit()`: the model quit and `Result().Started` is true.
- `TestQuitAfterTheEnd`: with an ended application `q` quits at once; the message line had the sentence with the status.
- `TestTheOutputIsSafe`: the fake's `Out` cannot hold unsafe text in a real run (termlog's promise), but the screen is checked all the same: no byte below `0x20` but the new line, with the sample.
- `TestEveryLineFits` gets the run screen, both tabs, at every size of step 5.2.
- `cmd/tube/tui_server_test.go` (tag): `TestTheTUIRunsPing` of step 5.5 changes: after `\r` on the profile, press `tab` (the Output tab) and read the master side until `bytes from 10.8.0.1` is on the screen; wait for `ended with status 0`; write `q`; the process ends with status 0. `TestQuitStopsTheApplication`: the target `sleep 300`; `q`, `y`: the process ends within 10 seconds with status 143 and no `sleep` of the test is left.

**Proof** (U), inside `scripts/vpn-ns.sh` with the server of step 5.5, as that step's proof does:
```
scripts/tui-proof.sh docs/implemented-proof-images/6.2-status.txt --config-in-place --config "$dir/cfg" --wait-first test --keys Enter --wait 'Received' -- ping -c 20 -i 0.5 10.8.0.1
scripts/tui-proof.sh docs/implemented-proof-images/6.2-output.txt --config-in-place --config "$dir/cfg" --wait-first test --keys Enter --wait 'Received' --keys Tab --wait 'bytes from 10.8.0.1' -- ping -c 20 -i 0.5 10.8.0.1
```
`6.2-status.txt` must show the Status tab with the address `10.8.0.2/24` and counters that are not zero; `6.2-output.txt` the lines of `ping`. The proof `5.5-ran.txt` of step 5.5 showed the old behavior; leave it as it is (a proof is a record of its step).

**Finish**: rules 10 to 15. Add to `docs/MANUAL-CHECKS.md`: `tube firefox` with a real profile: the Status tab's rates move while a video plays; the Output tab shows what Firefox prints; `q`, `y` closes Firefox.

### Step 6.3: The Network tab

**Goal**: a third tab where the user gives the running application another VPN, or takes the network away from it.

**Read**: SPEC §13.10, §10 ("Disconnecting and switching").

**Files**: `internal/tui/tab_network.go`, `run_screen.go`, `page_login.go`, `keys.go`, with tests.

**Tasks**

1. `tab_network.go`: the list of the profiles (the same rows as the profiles page, from the snapshot), with a mark in front of the one in use: `*` and the word `in use` in place of the "last used" column when the state is `connected`; `connecting again…` when it is `reconnecting`. Above the list, one line: `<application> uses Work.`, `<application> has no network: connecting again to Work…`, `<application> has no network: disconnected.`, or `<application> has no network: the server refused the login.` The selection starts on the profile in use.
2. Keys of this tab:
   - `enter` on a profile that is not in use, or on any profile when the state is not `connected`: switch to it. A profile that needs a password opens the login page of step 5.5 first (make the page take what to do after `enter`: connect, or switch; `esc` goes back to this tab). While the switch runs, the line above the list says `Connecting to <name>… <N> s   esc cancels` (the old VPN is still in use) and `esc` cancels the context of the call. The result: `Now using <name>.` on the message line; a refused login goes back to the login page with the sentence; another error shows the sentence, and the line above the list shows what the application has now.
   - `enter` on the profile in use while connected: the message `<application> already uses <name>.`
   - `x`: the question `Disconnect? <application> keeps running without network.` with `yes` = `Disconnect`; yes calls `Disconnect()`. When there is nothing to disconnect (the state is `disconnected`), `x` does nothing.
   - `up`, `down` and the other keys of the list; `q` and `ctrl+c` quit as on the Status tab; `?` the help page.
   - A broken profile cannot be chosen (the message says why).
3. After the application ended, `enter` and `x` show `<application> has ended.` and call nothing.
4. `run_screen.go`: the third tab, `Network`. `keys.go`: its rows.

**Tests** (with `apptest.Running()`)

- `TestGoldenNetwork`: at 100×30 and 60×16: connected (`Work` in use); reconnecting; disconnected; refused; while a switch runs.
- `TestSwitchFlow`: select `Home` (no password), `enter`: `Calls()` has `Switch(id-home, nil, false)`; the message is `Now using Home.` after the fake's `Run` changed and `Emit()`.
- `TestSwitchWithALogin`: start with `Home` in use; select `Work`, `enter`: the login page; type the password, `enter`: `Calls()` has `Switch(id-work, ada/***, true)`. `esc` on the login page goes back to the Network tab with no call.
- `TestSwitchFails`: `Fail["Switch"]` with a refused login: the login page again, the password empty; with another error: the sentence on the message line, the tab still shows `Work` in use.
- `TestCancelASwitch`: `Block["Switch"]`; `esc`: the call's context was cancelled; the message is `Cancelled.`
- `TestTheProfileInUse`: `enter` on `Work` while connected: no call, the message.
- `TestDisconnectFlow`: `x`, `y`: `Calls()` has `Disconnect()`; `x`, `enter`: no call; with the state `disconnected`, `x` shows no question.
- `TestConnectAgainAfterADisconnect`: with the state `disconnected`, `enter` on `Work` (the last profile): `Switch` is called.
- `TestABrokenProfile`, `TestAfterTheEnd`.
- `TestThePasswordIsNeverDrawn` of step 5.5 also runs for the login page opened from this tab.
- `TestEveryLineFits` gets this tab.
- `cmd/tube/tui_server_test.go` (tag): `TestDisconnectInTheTUI`: the target `ping -c 30 -i 0.5 -W 1 10.8.0.1`; when replies are on the Output tab, go to the Network tab, `x`, `y`; back on the Output tab no new `bytes from` line comes for three seconds; on the Network tab `\r` on the profile; replies come again.

**Proof** (U), as step 6.2's:
```
scripts/tui-proof.sh docs/implemented-proof-images/6.3-network.txt --config-in-place --config "$dir/cfg" --wait-first test --keys Enter --wait 'Received' --keys Tab --keys Tab --wait 'in use' -- ping -c 20 -i 0.5 10.8.0.1
scripts/tui-proof.sh docs/implemented-proof-images/6.3-disconnected.txt --config-in-place --config "$dir/cfg" --wait-first test --keys Enter --wait 'Received' --keys Tab --keys Tab --wait 'in use' --keys x --keys y --wait 'has no network: disconnected' -- ping -c 20 -i 0.5 10.8.0.1
```

**Finish**: rules 10 to 15. Add to `docs/MANUAL-CHECKS.md`: with two real profiles, `tube firefox`: a "what is my IP" page shows the first VPN's address; after a switch on the Network tab and a reload, the second's; after `x`, the page does not load; and it never shows the machine's own address. **Last step of M6**: `devVersion` becomes `0.8.0-dev`; tag `v0.7.0` with the message `M6: The TUI: the running application`; push.

---

## M7: Release (v0.8.0)

### Step 7.1: The documents and the release build

**Goal**: a user can read what Tube is, build it, use it, and know what it promises; a script builds the files of a release.

**Read**: the whole SPEC.

**Files**: `README.md`, `docs/GUIDE.md`, `docs/SECURITY.md`, `docs/MANUAL-CHECKS.md`, `scripts/build-release.sh`, `internal/archtest/release_test.go` (tag `tube_release`).

**Tasks**

1. `README.md`: what Tube is (SPEC §1, in the user's words); two examples (`tube firefox`, `tube --profile work curl https://example.com`); what it needs (§3), with the three settings of the kernel named; how to build and install (`go install github.com/rm4n0s/tube/cmd/tube@latest`, or `go build -o tube ./cmd/tube`); the limits (§17), each in one line; where the documents are; the license (GPL-2.0-only) and the note about the OpenVPN trademark that the file already has. No badge, no screenshot that was not made by `scripts/tui-proof.sh`.
2. `docs/GUIDE.md`, for the user, with these headings: **What Tube needs** (the kernel's settings, how to see them with `sysctl`, what the error looks like); **Adding a profile** (the keys; what "runs scripts" means; a profile with files next to it); **Running an application** (the three tabs: Status, Output, Network; typing into the Output tab; switching the profile and disconnecting while the application runs; quitting); **Firefox and other programs that are already open** (§8.7: why, `MOZ_NO_REMOTE`, `tube firefox -P <profile>`); **DNS servers of a profile**; **A password** (asked each time; `--auth-file` for scripts, mode 600); **Using Tube in scripts, and for programs that need the whole terminal** (`--profile`, `--quiet`, the exit statuses; `tube -p work ssh host`, `tube -p work htop`); **When the VPN drops** (what Tube writes; that the application has no network in between); **What the errors mean** (the sentences of `app.Explain`, each with what to do); **Where Tube keeps its files** (§5); **The log**.
3. `docs/SECURITY.md`: the table of SPEC §14 in plain words, a section "What Tube does not promise" (the last paragraph of §14, and §17), and how to report a problem (an issue on GitHub; for something that should not be public first, the address of the repository's owner on their GitHub profile).
4. `scripts/build-release.sh [--version <v>] [--dist <dir>]`, executable: the version is the argument, or `devVersion` without `-dev`; for `linux/amd64` and `linux/arm64`: `CGO_ENABLED=0 GOOS=linux GOARCH=<arch> go build -trimpath -ldflags "-s -w -X github.com/rm4n0s/tube/internal/app.buildVersion=<v>" -o <work>/tube ./cmd/tube`, then a `tube-<v>-linux-<arch>.tar.gz` with `tube`, `LICENSE` and `README.md` (made with fixed times and owners: `tar --sort=name --mtime=@0 --owner=0 --group=0 --numeric-owner`), in `<dist>/<v>/` (`dist` by default); then `SHA256SUMS` there (`sha256sum`, the file names without a folder). It uploads nothing and makes no tag. Exit status 64 for a mistake in its arguments.
5. `docs/MANUAL-CHECKS.md` is complete: read every item, add what only a real VPN provider or a real desktop shows and is not there yet (sound and video in a browser through the VPN; a long download; a suspend and resume of the laptop while an application runs through Tube; `tube` on a machine where user namespaces are off, to see the error), each with the exact command and what to look for.

**Tests**

- `release_test.go` (tag `tube_release`, run by hand and in this step: `scripts/heavy.sh --alone -- go test -tags tube_release -run Release ./internal/archtest`): runs the script with `--version 9.9.9 --dist <temp>`; both archives and `SHA256SUMS` exist; `sha256sum -c` passes; the `amd64` archive has exactly `tube`, `LICENSE`, `README.md`; on an `amd64` machine the unpacked `tube --version` prints `tube 9.9.9`.
- `internal/archtest` gets `TestTheGuideExplainsEveryError`: every sentence that `app.Explain` can return for a Tag of its table has its first five words somewhere in `docs/GUIDE.md` (export the table's sentences from `internal/app` for this test, as `app.Sentences() []string`).
- Read the three documents once more against the code: every command, key and file name in them exists.

**Finish**: rules 10 to 15. **Last step of M7**: `devVersion` becomes `0.9.0-dev`; tag `v0.8.0` with the message `M7: Release`; push. Then stop and tell the user: every step is done, and what is in `docs/MANUAL-CHECKS.md` waits for them.
