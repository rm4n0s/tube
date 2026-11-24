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
