# Tube: Implemented steps

The log of the finished steps of `docs/IMPLEMENTATION.md`, in order (its rule 11).

## Step 0.1: The module and the checks
- Date: 2026-11-25
- What was done: the Go module (`github.com/rm4n0s/tube`, Go 1.27.3); `internal/app/version.go` (`devVersion`, `Version`, the value of a release build); `cmd/tube` with `--version` and "not implemented yet" (status 125); `.golangci.yml`; `scripts/heavy.sh` and `scripts/checks.sh`; a first `README.md`.
- Checks: `scripts/checks.sh` passed (the error tags and the tests against the real server are skipped: their scripts come with steps 0.2 and 1.4)
- Proofs: none
- Manual checks: none
- Differences from the plan or the spec: none

## Step 0.2: The architecture tests and the error-tag tool
- Date: 2026-11-25
- What was done: `internal/archtest` (adapted from Cave's): `CheckBoundaries` for the rules 1 to 6 and 8 to 11 of SPEC §4.2 with a fixture tree that breaks each of them, and `ScanTags` with the `errortags` command and `scripts/error-tags.sh` for rule 7; `internal/core/errtest.Route`, the assertion of a route that tests use; `docs/ERROR-TAGS-UNTESTED.md` (empty); `github.com/rm4n0s/errors` v0.2.0.
- Checks: `scripts/checks.sh` passed (the tests against the real server are skipped: their script comes with step 1.4)
- Proofs: none
- Manual checks: none
- Differences from the plan or the spec: SPEC §4.2 got four more rules, which CLAUDE.md asks for and Cave checks too: no `import "C"` (8), the allowed dependencies of `go.mod` (9), no invisible characters in the source (10), `InsecureSkipVerify` only in tests (11). Rule 1 also keeps `internal/core` from importing `internal/app`; rule 5 lets the packages that only tests use (`vpntest`, `nstest`) start programs; rule 7 counts a Tag as asserted the way Cave's tool does. A Tag of an exception that does not exist is not an error (only an exception without a reason is).

## Step 0.3: Storage, clock, sanitize and logging
- Date: 2026-11-25
- What was done: `internal/core/storage` (the folders of SPEC §5 from the environment, `EnsureDir` with mode 0700 whatever the umask, `NewTempDir`, atomic writes with mode 0600, `ReadFile` with a limit) and `storage/storagetest.Isolate`; `clock` (system and fake); `sanitize.Display` and the new `sanitize.HostName`; `logging` (JSON lines, mode 0600, rotation at 1 MiB, an existing log narrowed to 0600); `scripts/isolated.sh`. Taken from Cave and changed for Tube: no instance lock, no Windows code, no `ParseLevel`.
- Checks: `scripts/checks.sh` passed (the tests against the real server are skipped: their script comes with step 1.4). `scripts/isolated.sh` was run by hand: `TUBE_CONFIG_DIR` is a folder of mode `drwx------` under `/tmp/tube-iso.*`, which is gone afterwards; `sh -c 'exit 7'` gives 7; a missing `--` gives 64; a fixture folder that does not exist gives 64.
- Proofs: none
- Manual checks: none
- Differences from the plan or the spec: `EnsureDir` makes each missing folder itself (`os.Mkdir`, then `Chmod` to 0700) instead of `os.MkdirAll`: with a umask of 0777 the parents that `MkdirAll` makes have mode 000 and cannot hold the next folder. This adds the Tags `StorageDirStatFailed` (a link to nothing at the folder's path) and `StorageNotADir` (a file at the folder's path), each with its test. Only the folders that `EnsureDir` makes and the folder itself are set to 0700; an existing parent keeps its mode.

## Step 1.1: The store of profiles
- Date: 2026-11-25
- What was done: `internal/core/profiles`: `Load` (the parser of gopenvpn-conn over an `os.Root`, recording the files a profile names and keeping the limits of 16 files of 1 MiB), `Store` with `Add` (parse, copy, parse the copy, write `meta.json` last), `List`, `Get`, `Rename`, `SetUsername`, `SetDNS`, `MarkUsed`, `Delete`, `CleanName`, `ParseDNS`, and `ProblemOf` for the parser's errors; `profiles/profilestest` (a CA, server and client certificates with ECDSA P-256 keys, and client profiles, made at test time); gopenvpn-conn v1.0.1.
- Checks: `scripts/checks.sh` passed (the tests against the real server are skipped: their script comes with step 1.4)
- Proofs: none
- Manual checks: none
- Differences from the plan or the spec:
  - `Add` does not make the path absolute, so there is no `ProfilePathInvalid`: a relative path works as it is (`os.OpenRoot` takes a relative folder). An empty path is `ProfilePathEmpty`.
  - There is no `ProfileMetaReadFailed`: a `meta.json` that cannot be read is the error of `storage.ReadFile`, returned untouched (trap 4). A `meta.json` of another version is `ProfileMetaNewer`, apart from `ProfileMetaInvalid` (one Tag, one place).
  - More Tags than the plan names, each with its test: `ProfileStatFailed` (the folder of a profile cannot be looked into), `ProfileCopyFolderOpenFailed` (the original's folder is gone before the copy). `ProfileMetaEncodeFailed` and `ProfileRandomFailed` cannot happen and are in `docs/ERROR-TAGS-UNTESTED.md`.
  - An id is 16 lowercase hexadecimal characters; capitals are refused. `List` skips entries of the store's folder that are not folders with such a name. `ErrNotFound` matches `ProfileNotFound` with `errors.Is`.
  - The test of the file limit uses 17 `ca` options (the parser lets `ca` repeat, and each names a file), so the real limit is tested, not a lowered one.
  - `Store` has an unexported hook, `afterLoad`, that the tests use to change the original between the parse and the copy.

## Step 1.2: Editing a profile, and profiles that name scripts
- Date: 2026-11-25
- What was done: `Store.Text` and `Store.ReplaceText`; `Store.Add` takes `AddOptions{WithoutScripts}` and refuses a profile that runs scripts with `ProfileRunsScripts` (metadata `"options"`), or stores it with each script line turned into `# tube: disabled: <the line>`; `withoutScripts`, which reads the profile once and parses its text again after each line it disables (16 at most); `loadWith`, which parses a text given in memory with the files of the folder.
- Checks: `scripts/checks.sh` passed (the tests against the real server are skipped: their script comes with step 1.4)
- Proofs: none
- Manual checks: none
- Differences from the plan or the spec:
  - `ReplaceText` parses the new text in memory with `loadWith`, then writes it with `storage.WriteFileAtomic`. No candidate file, no rename, so no `ProfileTextReplaceFailed`: an atomic write already leaves the old text when it fails.
  - `Add` reads the profile's text once (in `withoutScripts`) and stores that text, so a change of the original after the parse does not reach the store. Reading the profile is now Tube's own error (`ProfileUnreadable`, `ProfileFolderUnreadable`), not the parser's. The test of step 1.1 that changed the original before the copy now damages a named file, which the parse of the copy still catches.
  - `Text` and `ReplaceText` refuse a broken profile (`ProfileBroken`), as SPEC §6.1 says a broken profile can only be deleted.
  - The names of the disabled options are given once each (two `up` lines give `up` once).

## Step 1.3: The tunnel
- Date: 2026-11-25
- What was done: `internal/core/vpn`: `Dial` (parses the stored profile, gives it the credentials, connects with gopenvpn-conn), the `Tunnel` interface and the real tunnel, `Network` and `NetworkFromState` (at most one address of each family, IPv4 first; the MTU from 576 to 65535; at most 8 DNS servers and 6 search domains, only host names), `KindOf` for the errors of a connection. `vpn/vpntest.Tunnel`, a tunnel in memory with `Deliver`, `Written`, `Fail`, `OnWrite`, deadlines and the MTU check.
- Checks: `scripts/checks.sh` passed (the tests against the real server are skipped: their script comes with step 1.4)
- Proofs: none
- Manual checks: none
- Differences from the plan or the spec: the tests of a server that does not answer use a UDP socket of the loopback that is bound and never answers, instead of a port where nothing listens: a closed port answers with an ICMP refusal at once, so a cancel after 100 ms would come too late to be tested. The test of the cancel shows that gopenvpn-conn's error carries `context.Canceled`, so `KindOf` needs nothing more.

## Step 1.4: The real server in tests
- Date: 2026-11-25
- What was done: `scripts/vpn-ns.sh` (a private user and network namespace with its loopback up, `TUBE_NS=1`); `vpn/vpntest.StartServer` (tag `tube_server`): a real `openvpn` server on the loopback of the namespace with keys of `profilestest`, pushed DNS and search domain, an optional user name and password checked by a script of the test, optional IPv6, `Stop` and `Start`, and a client profile for it; `internal/core/packettest` (ICMP echo requests and replies for IPv4 and IPv6, built by hand); the tests that dial the real server with `vpn.Dial` and ping it through the tunnel over UDP, TCP and IPv6, and with a right and a wrong password.
- Checks: `scripts/checks.sh` passed, with the tests against the real server (OpenVPN 2.7 of the machine). `pgrep -a openvpn` shows nothing afterwards.
- Proofs: none
- Manual checks: none
- Differences from the plan or the spec:
  - The server tests run one package at a time (`go test -p 1`, in `scripts/checks.sh`, `CLAUDE.md` and rule 10): every server uses 10.8.0.0/24, and two servers of two packages at once would share the namespace's routes, so the kernel could send one server's replies to the other's device.
  - `scripts/checks.sh` also lints with `--build-tags tube_server`, so the files of the server tests are linted too.
  - `packettest` has the IPv6 echo and the reading functions (`Src`, `Dst`, `Version`) now, which step 2.3 would have added: the IPv6 test of this step needed them.
  - The test of `vpn` that uses the server is in the external package `vpn_test`: `vpntest` imports `vpn`.
  - `Server.Running` was added, for the test of `Stop` and `Start`.

## Step 2.1: Netlink
- Date: 2026-11-25
- What was done: `internal/core/netlink` (Linux only): `Open`, `LinkUp`, `SetMTU`, `AddAddr` (IPv6 without duplicate address detection), `DelAddr`, `AddDefaultRoute` (a route that exists is not an error), `Links`, `LinkIndex`, `Addrs`, `Routes`, over one socket of rtnetlink, with the messages built by hand in the machine's byte order; `internal/core/nstest` (`Main`, `Run`, `Require`): a function of a test run in a new user and network namespace as uid 0; `golang.org/x/sys` is a direct dependency.
- Checks: `scripts/checks.sh` passed, with the tests against the real server. The tests of `netlink` and `nstest` also passed three times in a row under the race detector.
- Proofs: none
- Manual checks: none
- Differences from the plan or the spec:
  - One private function, `roundTrip`, does the exchange of both `do` (an acknowledged request) and `dump`, so that each Tag is created in one place; the plan's `ack(messages, seq)` is `ackErrno(message)`, which reads the code of one `NLMSG_ERROR` message.
  - More Tags, each tested: `NetlinkAddressInvalid` (a prefix that is not valid or has a zone) and `NetlinkFamilyInvalid` (a default route of a family that is neither IPv4 nor IPv6). `NetlinkSocketFailed` and `NetlinkBindFailed` cannot be caused and are in `docs/ERROR-TAGS-UNTESTED.md`. `NetlinkReceiveFailed` and `NetlinkParseFailed` are tested through an unexported hook that replaces the read of the socket.
  - Two bugs were found by the tests and fixed: a dump that took more than one read kept messages pointing into a buffer that the next read overwrote (the IPv4 address came back as `/0`); an answer shorter than a netlink header made `roundTrip` read for ever (now `NetlinkParseFailed`).

## Step 2.2: The keeper and the TUN device
- Date: 2026-11-25
- What was done: `internal/core/sandbox`: the messages between Tube and the keeper (JSON, one per packet of a `SOCK_SEQPACKET` socket pair, a descriptor with `SCM_RIGHTS`); `openTun`; the keeper (`KeeperMain`, `IsKeeper`), which makes its mounts private, makes `tube0`, sends its descriptor with `ready` and serves until the socket closes; `Start`, `Device`, `Close` and `Check` on Tube's side, starting `/proc/self/exe` as `tube-keeper` in new user, network and mount namespaces; `cmd/tube` runs the keeper when it is started as one.
- Checks: `scripts/checks.sh` passed, with the tests against the real server. The sandbox tests also passed twice under the race detector. No keeper is left after the tests.
- Proofs: none
- Manual checks: none
- Differences from the plan or the spec:
  - More Tags than the plan names, each tested: `SandboxStartCancelled`, `SandboxDeadlineFailed` (a receive on a socket that this side closed), `SandboxMessageTooLarge`, `SandboxSendFailed`, `SandboxProtocolError`, `SandboxKeeperExit`. Five that cannot be caused are in `docs/ERROR-TAGS-UNTESTED.md` with their reasons.
  - `message` has only the fields this step uses; steps 2.3 to 2.5 add theirs. A descriptor that comes with a message is made close-on-exec and non-blocking inside `receive`, so a descriptor that cannot be is a message that cannot be read (`SandboxMessageInvalid`), not one more Tag.
  - The test binary also plays three misbehaving keepers (chosen by `TUBE_FAKE_KEEPER`), to test `SandboxProtocolError`, `SandboxKeeperFailed` and `SandboxKeeperExit`.
  - `Close` does its work in a named method, `close`: under the race detector the closure's frame is named `Close.1`, not `Close.func1`, and the route test broke. Trap 3 of `docs/IMPLEMENTATION.md` now says this.

## Step 2.3: Addresses, routes and the application
- Date: 2026-11-25
- What was done: the keeper configures the namespace (`configure.go`: the loopback, IPv6 off on `tube0` without an IPv6 address, the MTU and the addresses, a default route for each family, `ping_group_range`) over one netlink socket, and answers `configure` at any time; it starts the application on `run` in a nested user namespace as the user's own uid and gid, and sends `exited` with the code or the signal. Tube's side: `Configure`, `Run`, `Wait` and `ExitStatus`, with one reader goroutine that hands the keeper's answers to the request in flight. `sandboxtest`: the `Peer` that plays the VPN on the device, and the probe `report`.
- Checks: `scripts/checks.sh` passed, with the tests against the real server (the tests of `sandbox` pass inside `scripts/vpn-ns.sh` too). No keeper or application is left after the tests.
- Proofs: none
- Manual checks: none
- Differences from the plan or the spec:
  - `receive` has no deadline any more: on Tube's side one goroutine reads every message of the keeper, and the wait for an answer (`Sandbox.exchange`) has the step's timeout, so `SandboxKeeperSilent` is created there; `SandboxDeadlineFailed` is gone. Messages carry a sequence number (`seq`), which the keeper puts into its answer, so that a late answer is never taken for the answer to a later request.
  - A `failed` message carries the route of the keeper's error, which Tube puts into the metadata `keeper_route`: the tests assert the keeper's routes through it (`keeper.startApp.KeeperAppNotFound`, ...).
  - More Tags, each tested: `SandboxCancelled` (the context ended while Tube waited for an answer), `KeeperNoApplication` (a `run` to a keeper started only as a check). `KeeperAppStartFailed` is tested with a `run` of a uid that cannot be mapped.
  - `writeSetting` opens the existing file of the setting: `os.WriteFile` would have created a file that does not exist, and the test of a missing file found it.
  - Until step 2.5, the keeper kills the application with SIGKILL when the socket closes (step 2.5 makes it SIGTERM, then SIGKILL after the kill delay): no application is left behind in the meantime.
  - `packettest` had what this step needed already (step 1.4). Tests that the plan does not name: `TestRunWithoutAnApplication`, `TestRunRefusesBadIDs`, `TestAppCannotStart` (the first part of step 2.5's tests of "not found" and "not executable", which the new Tags needed), `TestWriteSetting`, `TestCancelled`, `TestKeeperLost`, `TestALateAnswerIsDropped`.

## Step 2.4: The names
- Date: 2026-11-25
- What was done: `ResolvConf` (at most 3 servers, 6 valid search domains, `options edns0`, a comment when there is no server); the keeper's `dnsFiles`, which on the first `configure` follows `/etc/resolv.conf` by hand, covers `/run/systemd/resolve`, `/run/nscd`, `/var/run/nscd` and `/run/avahi-daemon` with an empty `tmpfs`, and on every `configure` writes the file into the covered folder or into a file of the keeper's folder that is bind-mounted over it (rewritten in place later); `Configure` returns the warnings. `dnstest` (`Query`, `Question`, `Answer`), `packettest.UDP4` and `ParseUDP4`, `Peer.AnswerDNS`, and the probes `lookup` and `files`.
- Checks: `scripts/checks.sh` passed, with the tests against the real server. `getent hosts` inside the namespace gets its answer from the peer, through the device.
- Proofs: none
- Manual checks: none
- Differences from the plan or the spec:
  - `Options.TempDir` reaches the keeper as `TUBE_KEEPER_TMP`. `Start` removes every `TUBE_KEEPER_*` variable from the environment it is given, and the keeper removes them from the application's: the user's environment cannot steer the keeper, and the application does not see them.
  - An unexported option, `resolvConf`, gives the keeper another file in place of `/etc/resolv.conf` (`TUBE_KEEPER_RESOLV`), so that a machine with a plain file is tested here too (`TestResolvConfMountedOverAFile`), and a machine with none (`TestNoResolvConf`).
  - More Tags, each tested: `KeeperResolvLinkFailed` (a loop of links), `KeeperNoTempDir` (a file must be mounted and `Options.TempDir` is empty).
  - `writeResolv` writes with `os.WriteFile`, which truncates and writes the same file, so the bind-mounted file keeps its inode (trap 16).
  - The tests of `dnstest` compare the bytes with ones written by hand: `golang.org/x/net/dns/dnsmessage` is not an allowed dependency.

## Step 2.5: Signals and the ways an application ends
- Date: 2026-11-25
- What was done: the keeper catches SIGINT and SIGQUIT and drops them, passes SIGTERM and SIGHUP on to the application, sends the signal of a `signal` message, and when the socket closes ends the application with SIGTERM and, after the kill delay, SIGKILL. `Sandbox.Signal`; `Options.Terminal` (the application leads a new session on its own terminal); `LookPath` and `ExitCodeOf`; `internal/core/pty` (`Open`, `SetSize`); the probes `stubborn` and `sigint`.
- Checks: `scripts/checks.sh` passed, with the tests against the real server (the tests of `sandbox` and `pty` pass inside `scripts/vpn-ns.sh` too). After every test of the signals, no child of the test process is left.
- Proofs: none
- Manual checks: none
- Differences from the plan or the spec:
  - `LookPath` of a name with a slash only makes it absolute (`filepath.Abs`): `exec.LookPath` would refuse a file that cannot be executed as "not found" (127), and the shell's answer for it is 126, which the keeper gives (`SandboxAppNotExecutable`). A name found in a relative folder of `PATH` (`exec.ErrDot`) is not found. The cause of `SandboxLookPathFailed` is not kept: its text has the name as it is.
  - `pty.SetSize` reaches the descriptor through `SyscallConn`, so that the master side stays non-blocking and keeps its deadlines (`File.Fd` would make it blocking); a test asserts it.
  - `PtyNonblockFailed` is one more Tag. The Tags of `pty.Open` cannot be caused and are in `docs/ERROR-TAGS-UNTESTED.md`.
  - Tests that the plan does not name: `TestSigtermToTheKeeperIsPassedOn` (SIGHUP to the keeper reaches the application), `TestSetSizeFails`; `TestAppCannotStart` (step 2.3) checks `ExitCodeOf` too. `TestTheKeeperSurvivesCtrlC` sends SIGQUIT as well as SIGINT.

## Step 2.6: The pump
- Date: 2026-11-25
- What was done: `internal/core/pump`: two goroutines move packets between the device and the tunnel; a packet that the tunnel refuses for its size or that the device refuses (`EINVAL`, `EIO`, `ENOBUFS`, `EAGAIN`) is dropped and counted; a failure of the tunnel ends the pump with the tunnel's error, untouched, and a failure of the device with `PumpDeviceReadFailed` or `PumpDeviceWriteFailed`; `Stop` interrupts both reads with a deadline in the past, waits, and clears the deadlines so that the device and the tunnel can be used again; the counters can be read while it runs.
- Checks: `scripts/checks.sh` passed, with the tests against the real server. The tests of `pump` also passed three times in a row under the race detector.
- Proofs: none
- Manual checks: none
- Differences from the plan or the spec:
  - `Counters` has a named type for each way, `Traffic{Packets, Bytes}`, instead of an anonymous struct, so that tests and callers can write the values.
  - The fake device of the tests is a `vpntest.Tunnel` (it has the queue, the failures and the deadline that a device needs) with a `Write` that can be made to fail.
  - `Err` keeps the error of a pump that ended by itself after `Stop` is called; it is nil when `Stop` ended the pump.

## Step 2.7: The session
- Date: 2026-11-25
- What was done: `internal/core/session`: `Start` makes the temporary folder and the namespace (with `TUBE=1`, `TUBE_PROFILE`, `MOZ_NO_REMOTE=1` and the extra variables, each replacing one of the same name), configures it, writes the warnings as lines, starts the pump and the application, and undoes everything (and closes the tunnel) when a step fails; a watcher writes a line when the VPN connection ends while the application runs, with a reason made from the kind of the error; `Wait` cleans up and returns the exit status; `Signal`. The probes `ping-twice` and `linger`.
- Checks: `scripts/checks.sh` passed, with the tests against the real server: a ping through a real OpenVPN server, the session and the namespace works over IPv4 and IPv6.
- Proofs: none
- Manual checks: none
- Differences from the plan or the spec:
  - The two tests against the real server are one table test, `TestThroughARealServer` (IPv4 and IPv6).
  - The probes that talk to their test through files get the names of the files from `sandboxtest.ReadyFileVar`, `GoFileVar` and `OutFileVar`; `linger` writes `ready <pid>`, so that the test finds the application and, through `/proc/<pid>/stat`, the keeper, and waits for both to be gone.
  - A `Session.Signal` that does not reach the application is logged at the debug level.
  - `TestReason` checks the words of the line about the end of the VPN.

## Step 3.1: Connecting and explaining
- Date: 2026-11-25
- What was done: `app.App`: `Open` (the folders, the log with `tube started`, the store), `Close`, `Target`, `FindProfile` (the exact name, else the only one equal without case; not found, ambiguous, broken), `Connect` (the dial, then `MarkUsed` and, with `remember`, the user name; a failure is logged as `connect failed`), `Run` (`LookPath`, `session.Start`, `Wait`, with `run started` and `run ended` in the log), `Signal`. `app.Explain` and `app.ExitCode`.
- Checks: `scripts/checks.sh` passed, with the tests against the real server: a ping through the real server from `App.Run`, a remembered user name, and the sentence of a wrong password.
- Proofs: none
- Manual checks: none
- Differences from the plan or the spec:
  - The rows of `TestExplain` for the parser's problems and for the kinds of a failure of a connection are errors with the library's Tag strings (`"UnsupportedOption"`, `"AuthFailed"`, ...) written in the test: `internal/app` may not import gopenvpn-conn, in tests too (SPEC §4.2 rule 3). One row is a real profile that the parser refuses (`comp-lzo`), and the name that is taken is a real `Rename`.
  - `ProblemSyntax` without a line says `The profile cannot be read: a mistake.`; `sandbox.TagAppNotFound` (which has no name) says `The program was not found.`
  - The profiles `HOME` and `home` of `TestFindProfile` are made by writing the name into `meta.json` by hand: the store refuses two names that differ only by case.
  - `Run` returns an exit status with its error too (125, or 127/126 for an application that is not found or cannot be executed), so that the caller does not need `ExitCode` for these; `TestConnectNeedsCredentials` also checks that a password given to a server that does not answer stays out of the log, and that a failed connect changes nothing of the profile.

## Step 3.2: `tube --profile`
- Date: 2026-11-25
- What was done: the command line of SPEC §11 (`flags.go`, `usage.go`); the credentials from `--auth-file` (mode 0600 or narrower, two lines) or asked on `/dev/tty` with the echo off (`password.go`); `run` in `main.go`: the options, the early `LookPath`, `sandbox.Check`, `app.Open`, `--profile` (find, credentials, a connect that SIGINT, SIGTERM and SIGHUP cancel with status 130), and the run, during which SIGINT and SIGQUIT are caught and dropped and SIGTERM and SIGHUP are passed on to the application. Every message of Tube is one line on standard error that starts with `tube: `. `vpntest.StartDNS`. `docs/MANUAL-CHECKS.md`.
- Checks: `scripts/checks.sh` passed, with the tests against the real server: the built `tube` pings through a real OpenVPN server, ends with the application's status, finds names through the VPN with `getent`, refuses a wrong password with the sentence of `AuthFailed`, ends with 127 before connecting when the application is not there, and ends with 143 on SIGTERM with nothing left.
- Proofs: none
- Manual checks: pending, see docs/MANUAL-CHECKS.md
- Differences from the plan or the spec:
  - The echo of the terminal is switched off before `Password: ` is written, so that nothing typed after the prompt is ever echoed; the test types each answer only after its prompt is shown, as a user does.
  - More Tags, each tested: `TubeTerminalWriteFailed` (a prompt cannot be written) and `TubeTerminalLineTooLong` (more than 4 KiB without a new line); `TubeTerminalReadFailed` is a read that fails otherwise. `openTerminal` takes the path of the terminal, so that `TubeNoTerminal` is tested.
  - The hint after a mistake in the arguments is a line of Tube too: `tube: Try 'tube --help'.`
  - The end-to-end tests are subtests of one `TestEndToEnd`, which builds the binary once into its temporary folder; the last subtest checks that no process with that folder in its command line is left.
  - `App.Signal` keeps a signal that comes while the application is being started (after the line `connected`, before the App holds the session) and passes it on as soon as the session exists: the end-to-end test of SIGTERM found the gap.
  - The manual check of this step needs a profile in the store, which only the TUI (step 5.3) can add for a user.

## Step 4.1: Connect again
- Date: 2026-11-25
- What was done: the session's watcher became a controller (`reconnect.go`): one goroutine that owns the tunnel and the pump and selects over the end of the pump, the end of a wait, the result of a redial and the end of the application. When the tunnel fails it says so, waits 1, 2, 4, 8, 15 and then 30 seconds on the session's clock, calls `Options.Redial`, configures the namespace for the new tunnel and starts a new pump (`tube: connected again`); a refused login stops the tries; the end of the application cuts a wait short and cancels a redial in flight (a tunnel that it still returns is closed). `App.Run` gives the session a `Redial` that dials the same profile again with the credentials of `Connect`, which the App keeps in memory until `Close`.
- Checks: `scripts/checks.sh` passed, with the tests against the real server: under `tube --profile test ping -i 1 -c 25`, the real server stops and starts again three seconds later, and the replies come back through the new connection.
- Proofs: none
- Manual checks: none
- Differences from the plan or the spec:
  - The probe `ping-twice` of step 2.7 became `ping-steps`, which pings each address of its arguments in turn, each after the test allows it (files `<go>.<i>` and `<ready>.<i>`); `TestReconnect` and `TestNoNetworkBetweenTheTunnels` use it with two and three addresses, and the test of step 2.7 with two. The probe `report` waits for the file of `GoFileVar` when it is set, so that it reports after the reconnect.
  - `TestReconnect` has two subtests: the pings, and the report of the new address with one default route.
  - The session keeps the traffic of all its pumps (`traffic`), which the end of the run logs; step 4.2 shows it in the status.
  - The end-to-end test of the restart is a subtest of `TestEndToEnd`, `the server restarts`.

## Step 4.2: Disconnecting, switching, and the session's status
- Date: 2026-11-25
- What was done: the controller of step 4.1 also takes the user's commands: `Disconnect` (the pump stops, the tunnel is closed, no tries) and `Use` (the tries stop, the old tunnel is closed, the namespace is configured for the new one and a new pump starts; when the namespace cannot be configured, the new tunnel is closed and the session stays disconnected; after the end, `SessionEnded`). `Status` (state, since, profile, network, DNS, the traffic of all the pumps, whether the application runs and its exit status), `Done`, and `Options.OnChange`, called by the controller after each change of the state and at the end of the application. The probe `ping-on-demand`.
- Checks: `scripts/checks.sh` passed, with the tests against the real server. The tests of `session` also passed with `-race -count=3`.
- Proofs: none
- Manual checks: none
- Differences from the plan or the spec:
  - The session keeps the connection in use in its own fields (`tunnel`, `pump`, the profile's name, its DNS servers, its `Redial`), which `Start` sets from the options and `Use` replaces; there is no separate first `Connection` value.
  - A tunnel that fails with no `Redial` leaves the session in `StateDisconnected` (nothing will connect it again until `Use`).
  - `State` has a `String` method, for the logs and the tests.
  - `Disconnect` returns when it is done, and a `Disconnect` or `Use` that comes after the application ended does nothing (`Use` closes the given tunnel and returns `SessionEnded`).
  - The status of a disconnected session keeps the DNS servers that `resolv.conf` still has, and has the zero network.
