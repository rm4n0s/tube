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

## Step 4.3: The log of a terminal
- Date: 2026-11-25
- What was done: `internal/core/termlog`: `Log` turns what a program writes to a terminal into plain lines: UTF-8 that is cut between writes is finished by the next one and invalid bytes become U+FFFD; `\n` and `\r\n` end a line; a lone `\r` lets the next text empty the line (a progress bar); backspace and tabs; every control character, format character and line or paragraph separator is dropped; escape sequences (CSI, OSC, DCS/SOS/PM/APC, ESC and one byte) are dropped, also when they are cut between writes, and one that has not ended after 256 bytes is given up. The lines are bounded in width and number; `Len`, `Range`, `OnWrite`.
- Checks: `scripts/checks.sh` passed, with the tests against the real server.
- Proofs: none
- Manual checks: none
- Differences from the plan or the spec:
  - A CSI that meets a byte which cannot be part of it ends there, and that byte is read as text (so a new line inside a broken sequence still ends the line).
  - The byte after the first 256 of a sequence that never ends is read normally; the test checks that none of the first 256 reaches a line.
  - `Log.String` joins all the lines, for logs and tests.
  - The first version of the test had the invisible characters it tests written as themselves; they are escapes now (SPEC §4.2 rule 10).

## Step 5.1: The snapshot and the commands
- Date: 2026-11-25
- What was done: `App.Snapshot` (the target made safe, the profiles as the TUI shows them, the clock's time); the commands of SPEC §12.2 (`AddProfile` with `~/` and the default name, `RenameProfile`, `SetUsername`, `SetDNS`, `DeleteProfile`, `BeginEdit`/`EndEdit` through a file of mode 0600 in the App's temporary folder, which `Close` removes); `Subscribe`, called after every change that worked, also when `Connect` marked the profile used or saved the user name; the `Application` interface, `IsRunsScripts` and `IsAuthFailed`; `apptest.Fake` with `Sample()` and `Empty()`.
- Checks: `scripts/checks.sh` passed, with the tests against the real server.
- Proofs: none
- Manual checks: none
- Differences from the plan or the spec:
  - SPEC §12.2 already said that the commands block, take a context and run inside a `tea.Cmd` of the TUI: task 5 had nothing to change.
  - The fake does not record `Snapshot` and `Subscribe` in `Calls`; its `AddProfile` returns the id `id-new`, `BeginEdit` the path `/edit/<id>.ovpn`, and `EndEdit` says that the profile changed.
  - `IsRunsScripts` makes the list of options safe with `sanitize.Display` (200 characters).

## Step 5.2: The frame, the list and the profiles page
- Date: 2026-11-25
- What was done: `internal/tui` on Bubble Tea v2.1.2 and Lip Gloss v2.1.1: `fit`, `clean`, `ago`, `wrap`; the `list`; one table of key bindings for the footer and the help page; the styles; the `Model` (snapshot, size, page, message line, `Result`); the frame (title, rules, page, message, keys; "The terminal is too small" below 40×10); the profiles page (the latest used selected first, the selection kept on the same profile across snapshots, broken profiles with their reason); the help page; `tui.Run` (subscription, ticker, a cancelled context is not an error). `cmd/tube` opens the TUI without `--profile`, after the lookup of the application, and refuses without a terminal (`TubeNotATerminal`). The fixture `scripts/testdata/three-profiles` and its generator; `scripts/tui-proof.sh`.
- Checks: `scripts/checks.sh` passed, with the tests against the real server. Every golden file was read.
- Proofs: `docs/implemented-proof-images/5.2-profiles.txt` (the header `select a profile to run true`, `Home`, `Lab` and `Work` with `password`, the footer) and `5.2-help.txt` (the keys and the three sentences); read, and they show what the step describes. No tmux or tube process was left.
- Manual checks: none
- Differences from the plan or the spec:
  - The list has a line of column names (`Name`, `Server`, `Last used`) above the rows.
  - The help page's three sentences are wrapped to the width (`wrap`), not cut.
  - `run` of `cmd/tube` takes `stdin` and `stdout` as `*os.File` and standard error as an `io.Writer`; the check of the terminal comes after the lookup of the application, so that a missing application is still 127 (SPEC §8.5), and is a function of its own, `checkTerminal`, so that its Tag has a route test.
  - `tui.Options` has an unexported size for a program without a terminal, for the tests of `Run`; `TuiRunFailed` is tested with an input that fails.
  - The tests of `internal/tui` assert routes with `HasRoute` themselves: the TUI may not import `errtest` (SPEC §4.2 rule 2, which covers test files).
  - The fixture's generator gives `profilestest` a `testing.TB` of its own that ends the program on `Fatalf`; it was run with `TZ=UTC`, so that the fixture says nothing of this machine's time zone.

## Step 5.3: Adding and deleting
- Date: 2026-11-25
- What was done: the text `field` (cursor, paste, limit, placeholder, secret); the `question` box drawn over the page, which takes every key (`y` yes; `n`, `esc`, `enter` no); the completion of paths (`complete`: `~`, common prefix by whole runes, folders with `/`, hidden names only when asked, names cleaned); the add page (path and name, `tab` completes, `enter` adds, `esc` goes back, the error of the app in a sentence until the next key, "Adding…" while it runs, the question for a profile that runs scripts and the add again without them on yes); `d` asks before deleting the selected profile. After an add, the new profile is selected and the message says "Added <name>."; after a delete, "Deleted <name>.". The TUI now cleans every text of a snapshot (names, servers, reasons, the application's arguments), the message line and the add page's error before it draws them.
- Checks: `scripts/checks.sh` passed, with the tests against the real server. Every golden file was read (`add-empty`, `add-candidates`, `add-error`, `delete-question`, and `help`, changed).
- Proofs: `docs/implemented-proof-images/5.3-add.txt` (the sentence of the parser for `comp-lzo` on line 3) and `5.3-delete.txt` (the question over the list, `[y] Delete   [n] Cancel`, the question's keys in the footer); read, and they show what the step describes. No tmux or tube process was left.
- Manual checks: none
- Differences from the plan or the spec:
  - The help page has one wrapped line per page ("Profiles — ↑↓ jk: move · …"), so that it fits 40×10 with the keys of the add page and the question.
  - The completion runs as a command (`completedMsg`), not inside `Update`, since it reads folders.
  - The test of a real `app.App` gives the folders through the App's environment (`TUBE_CONFIG_DIR`, `TUBE_STATE_DIR`, `XDG_RUNTIME_DIR`, `HOME`), not through `storagetest.Isolate`: the TUI may not import the core's helpers (SPEC §4.2 rule 2). Its broken profile is typed as `~/bad.ovpn` in that home folder, so the golden file does not hold a temporary path.
  - The test of hostile names of step 5.2 held U+FFFD where it meant ESC, and checked the screen with the styles stripped, so it did not see that the escape sequences of a name reached the terminal raw. It now uses real ESC and C1 bytes and checks the raw view; the TUI cleans the snapshot (above).

## Step 5.4: The profile's page and the editor
- Date: 2026-11-25
- What was done: `e` on the list opens a profile's page: `Name`, `Servers` (at most 8, then `… and N more`), `Login`, `User name`, `DNS servers`, `Last used`, `Files`, `Ignored options` (at most 3 lines), each cut to the width; a broken profile shows `Cannot be used` with its reason and takes only `d` and `esc`. `r`, `u`, `n` open one field under the lines (filled with the current value; `enter` saves, `esc` closes; an error leaves the field open with its text); `t` edits the text in the user's editor (`findEditor`: `$VISUAL`, `$EDITOR`, `nano`, `vi`, split at spaces, never a shell; `BeginEdit`, `tea.ExecProcess`, `EndEdit` even when the editor failed; `Saved.`, `Nothing changed.`, `The profile was not changed: …`, `No editor found. Set EDITOR.`); `d` deletes and goes back to the list; a profile that leaves the snapshot closes its page. `tui.Run` takes the editor's variables from `Options.Env` when it is set.
- Checks: `scripts/checks.sh` passed, with the tests against the real server. Every golden file was read (the eight `profile-*`, and `help` and `profiles-sample-40x10`, changed).
- Proofs: `docs/implemented-proof-images/5.4-profile.txt` (the page of `Home` with every label of task 1, the page's keys in the footer); read, and it shows what the step describes. No tmux or tube process was left.
- Manual checks: none
- Differences from the plan or the spec:
  - The footer's labels of the page are short (`u user`, `n DNS`, `t edit`), and a footer that does not fit with three spaces between the keys gets two; at 60 columns with an application the page's footer is still cut in its last key (`esc b…`), from 62 columns it fits.
  - When a field is open and the page is too short for the lines and the field, the lines are cut so that the field stays visible.
  - A text that was saved counts even when the editor then ended with an error; an editor that failed without a change shows `The profile was not changed: the editor failed: …`.
  - `enter` on the list stays "not built yet" until step 5.5; on the profile's page it does so only with an application.

## Step 5.5: Login, connecting and running
- Date: 2026-11-25
- What was done: `enter` on a profile, on the list or on its page, with an application: the login page for a profile with a password (`User name` filled with the remembered one, `Password` drawn as nothing, `[x] remember the user name`; the cursor in the password when a user name is filled in; `Give the user name and the password.`; `esc` goes back and forgets the password), the connecting page for another (`Connecting to <name>…  <N> s`, `esc cancels`: `Cancelling…`, then `Cancelled.` on the list or the login page), and `This profile cannot be used: <reason>` for a broken one. A refused password goes back to the login page with the user name kept and the password cleared; another error goes back to the list with its sentence. A connect that succeeds sets `Result().Run` and ends the TUI; `cmd/tube` then runs the application as with `--profile`. `ctrl+c` while connecting cancels and quits. Without an application, `enter` on the list opens the profile's page. `vpntest/cmd/server` (tag `tube_server`) starts the test server outside a test, for the proofs; `scripts/tui-proof.sh` has `--config-in-place`.
- Checks: `scripts/checks.sh` passed, with the tests against the real server, among them `TestTheTUIRunsPing` (the real binary in a pseudo-terminal: `enter` on `test`, then `64 bytes from 10.8.0.1`, status 0). Every golden file was read (`login-remembered`, `login-empty`, `connecting-0`, `connecting-12`, and `help`, changed).
- Proofs: `docs/implemented-proof-images/5.5-ran.txt` (inside the namespace: the profile added through the TUI, then `enter`: `tube: connected to test; running ping` and `2 packets transmitted, 2 received, 0% packet loss`) and `5.5-connecting.txt` (a profile whose remote is `127.0.0.1 9`, added and selected: `Connecting to Nowhere…  3 s`, `esc cancels`); read, and they show what the step describes. No openvpn, tmux or tube process was left.
- Manual checks: pending, see docs/MANUAL-CHECKS.md (5.5 a, b and c)
- Differences from the plan or the spec:
  - `vpntest/cmd/server` gives `StartServer` a small type that implements the methods of `testing.TB` that it uses (`Helper`, `Fatalf`, `Skip`, `TempDir`, `Cleanup`, `Logf`), over a nil `testing.TB` for the others: it changes nothing of `vpntest`. It copies the client profile and the files next to it into `--dir`.
  - The profile of `5.5-connecting.txt` is the fixture's `Home` with its `remote` line replaced by `remote 127.0.0.1 9`, so that it has a real `<ca>` block without a new program.
  - A connect that the user cancelled goes back even when it succeeded at the same moment; the App closes that tunnel at the next connect or when it closes.
  - The connecting page goes back to the profile's page (not the list) after a cancel, when it was started from there; after an error it goes to the list, as the plan says.
  - The help page lists the keys of the login and connecting pages.

## Step 6.1: Running the application under the App
- Date: 2026-11-25
- What was done: `App.Start` runs the application on a pseudo-terminal (`TERM=dumb`, the slave side as its terminal, the master side copied into a `termlog` of 5000 lines until EIO) and returns; `RunState` (the session's status, the rates of the last second from a sampler on the App's clock, the last 100 events made of Tube's lines without `tube: `, sanitized), `OutputLen`, `OutputRange`, `Input` (`AppInputFailed`), `ResizeOutput` (kept for a later Start too), `Disconnect`, `Switch` (dial first, then `session.Use`; a failed dial changes nothing), `Stop`, and `Finish` (stops, waits, closes the terminal, waits for the goroutines; `Close` calls it). The notifications of the output, the session, the events and the sampler are joined: at most one each 50 ms. `Connect` and `Switch` share one dial (logging, marking as used, remembering the user name); `Run` and `Start` share `takeTunnel`. The `Application` interface and `apptest.Fake` have the new methods (`Running()`, `Typed`, `Size`); `vpntest.ServerOptions` has `Subnet`.
- Checks: `scripts/checks.sh` passed, with the tests against the real server (`TestSwitch`, `TestSwitchToAWrongPassword`, `TestSwitchAfterDisconnect`, `TestServerSubnet`). The App's tests passed three times in a row with the race detector.
- Proofs: none (the step has no proof; the tests are its proof). No openvpn, tmux or tube process was left.
- Manual checks: none
- Differences from the plan or the spec:
  - SIGTERM and SIGHUP are now followed by SIGKILL five seconds later in the App (`killLater`), for `Stop` and for the signals of `--profile`, as SPEC §10 says; until now the keeper passed SIGTERM on and nothing followed it. `TestStopKills` shows it with a probe that ignores SIGTERM and a short delay.
  - `Finish` without `Start` returns 0 and no error (there is nothing to finish), so that `AppNotStarted` is created in one place, `Switch`.
  - The routes of `AppNotConnected` and `AppNoTarget` now go through `App.takeTunnel` (`App.Run->App.takeTunnel.AppNotConnected`), since each Tag is created in one place.
  - `TestSwitch` cannot show the refusal by a ping of the first server through the second tunnel: both test servers live in the namespace of `scripts/vpn-ns.sh`, whose kernel owns both tunnel addresses and answers either. It shows instead that the first server was told that the client left (`exit message received by peer`), and the refusal of the network after a disconnect is shown by `TestSwitchAfterDisconnect` (no answer).
  - `Finish` waits one second at most for the last output before it closes the terminal, so that a program the application left behind with the terminal does not keep Tube waiting.

## Step 6.2: The Status and the Output tabs
- Date: 2026-11-25
- What was done: after a connect, the TUI calls `Start` (the connecting page says `Connected to <name>. Starting the application…`) and shows the run screen: the header (the application, the profile, the state with its time, the rates; the rates go first, then the time, when it is narrow), the tabs `[Status]  Output` (`tab`, `shift+tab`, around the ends), the tab's page, the message line and the keys. The Status tab: the application, the VPN, the address, the DNS servers, the MTU, the traffic both ways, the dropped packets, and the newest events that fit. The Output tab: the window of the output (it follows the end; `pgup`, `pgdown`, `home`, `end`; `scrolled back · end: follow`; `No output yet.`), and every other key and pasted text go to the application. `q` or `ctrl+c` on the Status tab asks `<application> is still running. Stop it and quit?`; yes calls `Stop()`, says `Stopping <application>…` and quits when it has ended; a second `ctrl+c` quits at once; after the end, `<application> ended with status N. Press q to quit.` and `q` quits. `units.go` (`bytesText`, `rateText`, `durationText`, `countText`). `tui.Result` is `{Started}`; `cmd/tube` returns `App.Finish()`'s status after the TUI, and SIGTERM and SIGHUP end the TUI (then `Finish` stops the application). The help page has the keys of the two tabs.
- Checks: `scripts/checks.sh` passed, with the tests against the real server (`TestTheTUIRunsPing` now on the run screen, and `TestQuitStopsTheApplication`: status 143, no `sleep` left). Every golden file was read (the twelve `status-*`, the four `output-*`, the three `run-header-*`, and `help`, changed).
- Proofs: `docs/implemented-proof-images/6.2-status.txt` (the Status tab of `ping` through `test`: `10.8.0.2/24`, MTU 1500, 2 packets each way, the events) and `6.2-output.txt` (the Output tab with ping's `64 bytes from 10.8.0.1` lines); read, and they show what the step describes. No openvpn, tmux, tube or ping process was left.
- Manual checks: pending, see docs/MANUAL-CHECKS.md (6.2)
- Differences from the plan or the spec:
  - After the application ended, the header shows `<application> ended (status N) through <profile>` without a state or rates, and the Status tab's VPN line says `closed when the application ended` with `-` for the address, the DNS servers and the MTU: the session closes the VPN with the application.
  - The message line shows `Stopping <application>…` and the sentence of the end whenever nothing else is on it, so that a key does not clear them for good.
  - The Output tab's keys line after the end is `q: quit · tab: next tab · pgup pgdn: scroll`.
  - `tui.Run` returns the model's result also when its context was cancelled (Bubble Tea returns the model when it was killed), so that `cmd/tube` knows to call `Finish`.
  - At 60×16 the Status tab has no room left for events.

## Step 6.3: The Network tab
- Date: 2026-11-25
- What was done: the run screen's third tab, `Network`: the line of what the application has (`<application> uses Work.`, `… has no network: connecting again to Work…`, `… disconnected.`, `… the server refused the login.`, `… has ended.`), and the profiles with `*` and `in use` (or `connecting again…`) for the one in use; the selection starts on it. `enter` switches (through the login page of step 5.5 for a profile with a password, which then switches and goes back to this tab); while it runs, `Connecting to <name>… <N> s   esc cancels`, and `esc` cancels; then `Now using <name>.`, a refused login back on the login page, or the sentence of another error. `enter` on the profile in use says `<application> already uses <name>.`; a broken profile says why; `x` asks `Disconnect? <application> keeps running without network.` (nothing when there is nothing to disconnect); after the end, `enter` and `x` say `<application> has ended.` `q`, `ctrl+c` and `?` as on the Status tab. The help page has the tab's keys.
- Checks: `scripts/checks.sh` passed, with the tests against the real server, among them `TestDisconnectInTheTUI` (the real binary in a pseudo-terminal: replies on the Output tab; `x`, `y` on the Network tab; no reply for three seconds on the Output tab; `enter` on the profile; replies again). Every golden file was read (the nine `network-*`; the run screen's goldens and `help` changed only in the tabs line and the help's new line).
- Proofs: `docs/implemented-proof-images/6.3-network.txt` (`ping uses test.`, `>*test … in use`) and `6.3-disconnected.txt` (after `x`, `y`: `ping has no network: disconnected.`, the header `disconnected`); read, and they show what the step describes. No openvpn, tmux, tube or ping process was left.
- Manual checks: pending, see docs/MANUAL-CHECKS.md (6.3)
- Differences from the plan or the spec:
  - The mark of the profile in use is the second character of the row's mark: `>*` when it is also selected, ` *` when not.
  - `ctrl+c` on a page that the run screen opened (the login page of a switch, the help page) goes back to the run screen instead of quitting, so that the application is never left without the question of §13.11.
  - The row's last column is no longer padded to 12 cells, so that `connecting again…` fits; the profiles page looks the same.

## Step 7.1: The documents and the release build
- Date: 2026-11-25
- What was done: `README.md` (what Tube is, two examples, the first lines of a proof of the Status tab, what it needs with the three settings of the kernel, building and installing, the limits, the documents, the license and the trademark note); `docs/GUIDE.md` (the headings of the plan, with the keys of every page and tab, the exit statuses, and every sentence of `app.Explain` with what to do); `docs/SECURITY.md` (the promises of SPEC §14 and how each is kept and tested, what Tube does not promise, how to report a problem); `scripts/build-release.sh` (linux/amd64 and linux/arm64, `-trimpath`, the version set with `-X`, archives with fixed times and owners, `SHA256SUMS`; status 64 for a mistake). `app.Sentences()`, made from the table that `Explain` now uses for the Tags it knows. `docs/MANUAL-CHECKS.md` has four items more (sound and video, a long download, a suspend and resume, a machine without user namespaces).
- Checks: `scripts/checks.sh` passed, with the tests against the real server; `TestTheGuideExplainsEveryError` passes; `scripts/heavy.sh --alone -- go test -tags tube_release -run Release ./internal/archtest` passed (both archives, `sha256sum -c`, the amd64 archive holds `LICENSE`, `README.md` and `tube`, the unpacked `tube --version` prints `tube 9.9.9`, and two builds of one version are the same bytes); `golangci-lint run --build-tags tube_release` reports nothing. The three documents were read against the code: their commands, flags, keys and file names exist (`tube --help` and `tube --version` were run).
- Proofs: none (the step has no proof). No openvpn, tmux or tube process was left.
- Manual checks: pending, see docs/MANUAL-CHECKS.md (7.1 a to d)
- Differences from the plan or the spec:
  - `app.Sentences()` writes the pieces that come from an error as placeholders (`"<name>"`, `line <line>`), and also lists the sentences of the kinds of a failed connection and of the parser's problems, since the guide explains them too.
  - The tar command lists the files in their sorted order itself: `--sort=name` sorts only what tar finds in folders. The archives are compressed with `gzip -n`, so that they hold no time.
  - The item of step 6.2 in `docs/MANUAL-CHECKS.md` said to press `q` on the Output tab, where it goes to Firefox; it now goes back to the Status tab first. The item of step 3.2 now says how to add the profile in the TUI.
  - `TestReleaseBuild` also checks that two builds of one version are the same, and `TestReleaseRefusesAMistake` the status 64.
