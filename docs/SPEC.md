# Tube: Specification

Tube runs one application through a VPN, and only that application. This file is the source of truth for what Tube does. `docs/IMPLEMENTATION.md` breaks it into steps.

## 1. Purpose

- `tube firefox` or `tube ping -c 3 example.com` opens a terminal UI (TUI) with the user's **profiles** (OpenVPN client profiles, `.ovpn`). The user adds, looks at, changes and deletes profiles there, and selects one. Tube connects to it and starts the application.
- **The TUI stays open while the application runs.** It shows the state of the VPN and how much traffic passes (the Status tab) and what the application prints (the Output tab), and it lets the user give the application another profile or take the network away from it (the Network tab).
- **All the network of the application goes through the VPN**: TCP, UDP, ICMP and the looking up of names. Nothing else on the machine changes: other programs keep using the machine's network.
- **Nothing leaks.** The application has no way to the network but the tunnel. Before the VPN is connected and after it has ended, the application has no network at all.
- Tube needs no root, no `sudo`, no system service and no installed `openvpn`.
- When the application has ended and the user quits, Tube disconnects and exits with the application's exit status. Quitting earlier stops the application.
- `tube --profile <name> <application>` does the same without a TUI: the application gets the terminal, for scripts and for programs that need the whole terminal.

## 2. Decisions

| # | Decision | Source |
|---|---|---|
| D1 | The development process, the error handling and the way the TUI is built and tested are those of Cave's `CLAUDE.md` | the user, 2026-11-25 |
| D2 | A CLI tool for Linux: `tube <application> [arguments]` shows a TUI to add, read, change and delete `.ovpn` profiles and to select one, and all the network of the application goes through that VPN | the user, 2026-11-25 |
| D3 | The OpenVPN client is `github.com/rm4n0s/gopenvpn-conn` (packages `openvpn` and `ovpn`) | the user, 2026-11-25 |
| D4 | The program is called `tube`, as the repository | the user, 2026-11-25 |
| D5 | GPL-2.0-only, as gopenvpn-conn | the user, 2026-11-25 |
| D6 | A finished milestone is merged, tagged and pushed without asking, as in Cave | the user, 2026-11-25 |
| D7 | The application runs in a private user, network and mount namespace whose only interface besides the loopback is a TUN device. Tube, outside, moves IP packets between that device and the `*openvpn.Conn` | follows D2 ("all the network", so a proxy is not enough: `ping` is ICMP) and "no root". Shown to work on the development machine, 2026-11-25 |
| D8 | `vpnnet` (the userspace network of gopenvpn-conn) is not used | with D7 the kernel of the namespace is the TCP/IP stack; it also keeps gVisor (Apache-2.0) out of a GPL-2.0-only program |
| D9 | Go, cgo-free. The TUI is built on Bubble Tea v2 and Lip Gloss v2 (`charm.land/...`), and on nothing else of Charm (no `bubbles`): Tube's own list, text field and question are in `internal/tui` | follows D1 (Cave's D4) |
| D10 | The namespace always has a default route into the tunnel, for every address family the VPN gave an address for, whatever routes the server pushed | D2: all the network of the application; the namespace has nothing else to route to |
| D11 | A profile is stored as a copy: the profile file and the files it names, in Tube's configuration folder | a profile must keep working when the downloaded file is moved or deleted |
| D12 | A password is asked for each time and is never written to a file; the user name can be remembered | the safe default; change it when the user asks |
| D13 | `--profile <name>` runs the application with a profile without showing the TUI | scripts, programs that need the whole terminal (D19) and the tests against the real server need a way without a terminal UI |
| D14 | When the VPN connection ends by itself while the application runs, Tube connects again by itself; the application has no network in between. After the user disconnected (D17), Tube does not | a browser session outlives a short loss of the link; D2's "nothing else" still holds |
| D15 | Tube sets `MOZ_NO_REMOTE=1` for the application | without it `tube firefox`, with a Firefox already open, hands the page to the open one, which is outside the VPN (§8.7) |
| D16 | After a profile is selected, the TUI starts the application and stays open, with two tabs: one that shows the connection and its traffic (Status), one that shows the output of the application (Output) | the user, 2026-11-25 ("it will have two tabs in the TUI, one that show bandwidth etc and the other the output of the application") |
| D17 | A third tab (Network) lets the user select and use another profile, or disconnect the network altogether, while the application is running | the user, 2026-11-25 |
| D18 | In the TUI the application runs on a pseudo-terminal that Tube reads, with `TERM=dumb`. The Output tab shows what it prints as plain lines, and the keys typed on that tab go to the application | follows D16: the user's terminal belongs to the TUI, and a program whose output is a pipe holds its lines back; with the keys, Ctrl-C still stops `ping` |
| D19 | With `--profile` there is no TUI and the application gets the user's own terminal | D13; a full-screen program (`vim`, `htop`, `ssh`) needs a real terminal, which the Output tab is not |
| D20 | A switch to another profile connects the new one first and only then replaces the one in use; when the new one fails, nothing changes | the application keeps its network while the user waits; the gap is the moment of the replacement only |
| D21 | The TUI is colorful: its own colors for the name of the program, the selected row, the tabs, the states of the VPN, the columns, the keys and the buttons, for a dark and for a light terminal | the user, 2026-11-25 ("I expect to be more colorful") |
| D22 | The TUI is used with the mouse too: a click selects or presses, a double click opens, runs or uses, the wheel scrolls | the user, 2026-11-25 ("be able to use mouse") |
| D23 | The add page shows the folders and the `.ovpn` files of a folder as a list that is walked through with the mouse or the keys, and the file is chosen there | the user, 2026-11-25 ("when adding ovpn path, I want to show a file browser that I can surf with my mouse") |

## 3. Platform and requirements

- **Linux**, x86-64 and arm64. Nothing is written for another system (D2).
- A kernel that lets an ordinary user make user namespaces (`user.max_user_namespaces` above 0; on Debian `kernel.unprivileged_userns_clone=1`; on Ubuntu 24.04 and later `kernel.apparmor_restrict_unprivileged_userns=0` or an AppArmor profile for Tube), and `/dev/net/tun` that the user may open (mode 0666, the default of the distributions).
- Tube checks both when it starts (§8.8) and says what is missing; it never asks for root.
- A terminal for the TUI. `--profile` needs none.
- Go 1.27.3 or later (gopenvpn-conn's `go.mod`). No cgo anywhere.

## 4. Architecture

Three processes while an application runs:

```
the machine's network                      private namespace (user, network, mount)
┌──────────────────────────────┐           ┌─────────────────────────────────────────┐
│ tube                         │  socket   │ the keeper (tube again; root of the     │
│  TUI, profiles               │◄─────────►│  namespace only): makes the TUN device, │
│  *openvpn.Conn ── UDP/TCP ──►│ VPN       │  sets addresses, routes and DNS files,  │
│        ▲                     │ server    │  starts the application, waits for it   │
│        │ pump (IP packets)   │           │        │                                │
│        ▼                     │           │        ▼                                │
│  the TUN's file descriptor ◄─┼───────────┼─ tube0 ◄── the application (the user's  │
│                              │           │            own uid, no rights over the  │
└──────────────────────────────┘           │            namespace's network)         │
                                           └─────────────────────────────────────────┘
```

### 4.1 Packages

```
cmd/tube ──► internal/tui ──► internal/app ──► internal/core/...
        └────────────────────────┘
```

| Package | What it is |
|---|---|
| `cmd/tube` | `main`: the keeper's entry (§8.1), the flags (§11), then the TUI or `--profile` |
| `internal/app` | The application behind the TUI: owns the store, gives snapshots and commands (§12), connects, runs the session, and keeps the application's output. Also `version.go` and the sentences that errors are shown with |
| `internal/app/apptest` | Test support for the TUI: a fake application with fixed content (for golden files) |
| `internal/core/storage` | The folders, atomic writes and file modes (§5). `storage/storagetest` isolates tests |
| `internal/core/clock` | `Clock` (now, timers) and a fake for tests |
| `internal/core/sanitize` | `Display`: makes hostile text safe to store, log and draw (§6.5) |
| `internal/core/logging` | `slog` to a size-limited file (§15) |
| `internal/core/profiles` | The store of profiles: adding (copying), validating, listing, changing, deleting (§6) |
| `internal/core/vpn` | Connecting a stored profile with gopenvpn-conn; the `Tunnel` and `Network` types that the rest of the core uses (§7). The only package of the core that imports gopenvpn-conn, besides `profiles` (which imports `ovpn` to validate) |
| `internal/core/vpn/vpntest` | A fake tunnel in memory; with the `tube_server` tag, a real `openvpn` server on the loopback (§16.3) |
| `internal/core/netlink` | The few rtnetlink requests Tube needs: link up, MTU, addresses, routes (§8.3) |
| `internal/core/sandbox` | The namespace: the keeper and Tube's side of it, the TUN device, the DNS files, the application (§8) |
| `internal/core/pump` | Moves IP packets between the TUN device and the tunnel (§9) |
| `internal/core/session` | One run: a namespace, the application, and the tunnel and pump it has at the moment, with reconnecting, switching and disconnecting (§10) |
| `internal/core/pty` | A pseudo-terminal: the application's terminal when the TUI shows its output (§8.5) |
| `internal/core/termlog` | Turns what a program writes to a terminal into plain lines that are safe to draw (§13.9) |
| `internal/core/nstest`, `packettest`, `dnstest`, `profiles/profilestest`, `sandbox/sandboxtest` | Test support: a function run inside a namespace, IP and DNS packets built by hand, test profiles and keys, a peer that plays the VPN on the device |
| `internal/archtest` | Tests of the rules of §4.2, and the error-tag tool (`cmd/errortags`) |

### 4.2 Rules that `internal/archtest` checks

1. `internal/core/...` and `internal/app` import no UI library (`charm.land/...`, `github.com/charmbracelet/...`) and not `internal/tui`; `internal/core` does not import `internal/app`.
2. `internal/tui` imports `internal/app` and, of the core, only `clock` and `sanitize`. Nothing imports `internal/tui` but `cmd/tube`, and nothing imports a UI library but `internal/tui` and `cmd/tube`.
3. Only `internal/core/vpn` (with the packages below it) and `internal/core/profiles` import `github.com/rm4n0s/gopenvpn-conn/...`, in test files too; nothing imports its `vpnnet`.
4. Nothing imports the standard library `errors`, and no file but a test calls `fmt.Errorf` (CLAUDE.md "Error handling").
5. Nothing imports `os/exec` but `internal/core/sandbox` (which starts the keeper and the application), `internal/tui` (the editor, §13.5), test files, and the packages that only tests use (a folder whose name ends with `test`: `vpntest`, `nstest`, ...). Tube never runs `ip`, `unshare`, `openvpn` or a shell.
6. Every Go file starts with the line `// SPDX-License-Identifier: GPL-2.0-only`.
7. Every error Tag (the first argument of `errors.New`, `errors.NewErr`, `errors.NewErrf`: a string literal or a Tag constant) is created at one place and is asserted by a test, by its route (`HasRoute("...Tag")`) or by its Tag. `scripts/error-tags.sh --untested` lists the ones that are not; `docs/ERROR-TAGS-UNTESTED.md` is the only list of exceptions, each with its reason.
8. No file has `import "C"`.
9. `go.mod` requires directly only the modules that CLAUDE.md allows.
10. No Go file has a control or format character but the new line and the tab, or invalid UTF-8: such a character is written as an escape, so that the code a reviewer sees is the code the compiler gets.
11. `InsecureSkipVerify` appears only in test files.

## 5. Files and folders

| Folder | Where | What is in it |
|---|---|---|
| Configuration | `$TUBE_CONFIG_DIR`, else `$XDG_CONFIG_HOME/tube`, else `~/.config/tube` | `profiles/<id>/meta.json`, `profiles/<id>/files/...` |
| State | `$TUBE_STATE_DIR`, else `$XDG_STATE_HOME/tube`, else `~/.local/state/tube` | `tube.log`, `tube.log.1` |
| Temporary | a new folder under `$XDG_RUNTIME_DIR`, else under `os.TempDir()`, one per run, removed at the end | what the namespace needs (§8.4) |

- Every folder Tube makes has mode 0700 and every file mode 0600, whatever the umask. A folder that exists with wider rights is narrowed when Tube first writes into it.
- A file is written atomically: to a temporary file in the same folder, `fsync`, rename.
- There is no lock: several Tubes may run at once, each with its own VPN connection and namespace. The store reads the folder each time it lists; when two Tubes change the same profile, the last write wins.
- A relative path in a `TUBE_` variable is an error (`StorageDirNotAbsolute`); an XDG variable that is empty or relative is ignored, as the XDG specification says.

## 6. Profiles

### 6.1 What a stored profile is

A folder `profiles/<id>/`, where `<id>` is 16 hexadecimal characters from `crypto/rand`:

- `files/<name>.ovpn`: the profile as it was added (or as it was last edited), and next to it, at the same relative paths, the files that it names (`ca`, `cert`, `key`, `tls-auth`, `tls-crypt`, `tls-crypt-v2`, `auth-user-pass`, ...).
- `meta.json`:

```json
{"version": 1, "name": "work", "file": "work.ovpn", "created": "2026-11-25T10:00:00Z",
 "last_used": "2026-11-25T11:30:00Z", "username": "", "dns": ["10.0.0.53"]}
```

`name` is what the user sees. It is 1 to 64 characters after `sanitize.Display` and trimming, and no two profiles have names that are equal when compared without case. `username` is the remembered user name (D12); `dns` are the DNS servers that the user set for the profile (§8.4), IP addresses. A `meta.json` with a `version` that Tube does not know, or one that cannot be read, makes its profile appear in the list as broken, with the reason; it can only be deleted.

### 6.2 Adding

The user gives the path of a `.ovpn` file and a name (the default is the file's name without `.ovpn`).

1. The profile is parsed with `ovpn.ParseFS` over the folder of the file, opened as an `os.Root`, so a name that leaves the folder, also through a symbolic link, is refused. The file system that Tube passes records the names that the parser opens: those are the files of the profile.
2. Limits: the profile is at most 1 MiB (the parser's limit), each named file at most 1 MiB, and there are at most 16 of them (`ProfileFileTooLarge`, `ProfileTooManyFiles`).
3. The profile and the recorded files are copied to a new `profiles/<id>/files/`, and `meta.json` is written last: a folder with no `meta.json` is a failed add, and is removed the next time the store lists.
4. The copy is parsed again from the store. Only then is the profile added.

An error of the parser is shown with the option, the line and the reason that it carries (§12.3). A profile whose only problem is an option that runs a script (`up`, `down`, `up-restart`, `down-pre`, `route-up`, `route-pre-down`, `ipchange`, `tls-verify`) is offered again: "This profile runs scripts (up, down). Tube never runs them, and sets the DNS of the application itself. Add it without them?" On yes, the stored copy has each of those lines turned into a comment (`# tube: disabled: <the line>`); the line number comes from the metadata of the parser's error, one line for each try, at most 16 tries. The original file is never changed.

### 6.3 Changing

- **Rename**, **user name**, **DNS servers**: change `meta.json`. The DNS servers are IP addresses separated by spaces or commas, at most 4; empty means "the ones the VPN pushes".
- **Edit the text**: the profile file is copied to the temporary folder and opened in the user's editor (§13.5). When the editor ends and the text changed, it is parsed the way an add is, with the stored files next to it; when it is valid it replaces the stored profile file, and when it is not, the error is shown and the stored profile stays as it was.
- **Delete**: after a question, the folder of the profile is removed.

### 6.4 What the list shows

For each profile: the name, the first remote (`host:port` and `udp` or `tcp`) and how many more there are, whether it asks for a password (`NeedsCredentials`), and when it was last used. The profiles are sorted by name, without case. The list of options that the library ignores (`Profile.Ignored`) is shown on the profile's page.

### 6.5 Hostile text

A profile, and everything a VPN server sends, is hostile text. `sanitize.Display(s, max)` replaces every unsafe character with U+FFFD (control characters, so also the escape character and the new line; Unicode format characters such as the bidirectional controls; the line and paragraph separators; invalid UTF-8) and cuts to `max` characters, the last being `…` when it cut. Everything from a profile or a server passes through it before it is stored in `meta.json`, logged or drawn. A secret is never drawn, also not sanitized: the TUI shows of a profile only what §6.4 lists.

## 7. The VPN connection

`vpn.Dial(ctx, store folder of the profile, credentials)` parses the stored profile (as §6.2 step 1), sets `Config.Credentials` when the profile needs them, and calls `Profile.Dial`. It returns a `vpn.Tunnel`:

```go
type Tunnel interface {
    Read(packet []byte) (int, error)   // one IP packet from the VPN
    Write(packet []byte) (int, error)  // one IP packet to the VPN
    SetReadDeadline(t time.Time) error // interrupts a Read, so that the pump can be stopped
    Network() Network                  // what the server pushed, checked
    Close() error
}

type Network struct {
    Addrs         []netip.Prefix // the tunnel's addresses: at most one IPv4 and one IPv6
    DNSServers    []netip.Addr   // pushed by the server
    SearchDomains []string       // pushed by the server, only valid host names
    MTU           int            // 576 to 65535
}
```

- `Network` is made from `ConnectionState` and checked: no address is an error (`VPNNoAddress`); an MTU outside the range is an error (`VPNBadMTU`); a search domain that is not a host name (letters, digits, hyphens and dots, at most 253 bytes) is dropped, because it would be written into a file of the namespace.
- A profile that needs credentials and has none given: `VPNCredentialsNeeded`. The caller asks the user (§13.6, §11).
- The errors of gopenvpn-conn are returned untouched; §12.3 says how they are shown.
- Tube never falls back: when `Dial` fails, the application is not started.

## 8. The namespace

### 8.1 Starting the keeper

Tube starts itself again (`/proc/self/exe`, with `tube-keeper` as `argv[0]`; `main` looks at `argv[0]` first) with `CLONE_NEWUSER | CLONE_NEWNET | CLONE_NEWNS`, the user's uid and gid mapped to 0, `setgroups` denied, and one end of a `socketpair(AF_UNIX, SOCK_SEQPACKET)` as file descriptor 3. The keeper inherits the terminal, the environment and the working folder.

The keeper and Tube talk over the socket, one JSON message in each packet:

| From | Message | Meaning |
|---|---|---|
| keeper | `ready` + the TUN's file descriptor (`SCM_RIGHTS`) | the namespace exists; here is the device |
| Tube | `configure` (a `Network`, the DNS servers to use) | set, or set again after a reconnect, the addresses, routes and DNS files |
| keeper | `configured`, or `failed` (tag, message, metadata) | |
| Tube | `run` (the user's uid and gid, the delay before SIGKILL, whether the application gets a terminal of its own) | start the application, which the keeper was started with (its path and arguments are the keeper's own arguments) |
| keeper | `started` (pid), or `failed` | |
| Tube | `signal` (number) | send this signal to the application |
| keeper | `exited` (exit status, or signal) | the application ended; the keeper exits |

When the socket closes (Tube died), the keeper sends SIGTERM to the application, SIGKILL five seconds later, and exits. Tube's side of every step has a deadline of 10 seconds (`SandboxKeeperSilent`).

### 8.2 The TUN device

The keeper opens `/dev/net/tun` (`IFF_TUN | IFF_NO_PI`, named `tube0`), sends the file descriptor to Tube and closes its own. Tube holds the only reference: when Tube ends, however it ends, the device is gone, and the namespace has the loopback and nothing else.

### 8.3 Addresses and routes

On `configure` the keeper, through rtnetlink (`internal/core/netlink`, never the `ip` command):

1. brings the loopback up (the application has its own `127.0.0.1` and `::1`);
2. sets the MTU of `tube0`, removes the addresses it has, adds the ones of the `Network`, and brings it up;
3. adds a default route into `tube0` for IPv4 when there is an IPv4 address, and for IPv6 when there is an IPv6 address (D10). With no IPv6 address, IPv6 is switched off on `tube0` (`net.ipv6.conf.tube0.disable_ipv6=1`), so that the kernel sends nothing of its own into the tunnel;
4. writes `0 0` to `net.ipv4.ping_group_range` of the namespace, so that `ping` works without a capability.

### 8.4 The names (DNS)

The DNS servers of a run are the ones the user set for the profile (§6.3), else the ones the server pushed. Inside the mount namespace, after making every mount private:

1. The folders through which the C library reaches a resolver of the machine are covered with an empty `tmpfs`, when they exist: `/run/systemd/resolve`, `/run/nscd`, `/var/run/nscd`, `/run/avahi-daemon`. With the usual `nsswitch.conf`, `resolve` and `mdns` then are unavailable and the lookup goes on to `dns`.
2. `/etc/resolv.conf`, wherever its symbolic link leads, has this content inside the namespace: a `nameserver` line for each DNS server (at most 3, the C library's limit), a `search` line when there are search domains, and `options edns0`. When the file's place is in a covered folder it is written there; else a file of the temporary folder is bind-mounted over it.
3. With no DNS server at all the file has no `nameserver` line and a comment that says so; Tube warns before it starts the application ("the VPN gave no DNS server: names will not be found; set DNS servers for the profile"). The C library then asks `127.0.0.1`, where nothing listens.

`/etc/hosts` stays the machine's. What is outside these paths is in §17.

### 8.5 The application

On `run` the keeper starts the application in a nested user namespace (`CLONE_NEWUSER`) in which the user's own uid and gid are mapped back: the application sees itself as the user, owns its files as always, and has no capability over the network namespace, so it cannot change the addresses, the routes or the device. It gets the keeper's working folder and environment, with `TUBE=1`, `TUBE_PROFILE=<name>` and `MOZ_NO_REMOTE=1` added (D15).

Its terminal depends on how Tube was started:

- **With `--profile`** (D19) the application gets Tube's own terminal and stays in Tube's process group, so the keys of the terminal (Ctrl-C, Ctrl-\, Ctrl-Z) reach it as they would without Tube.
- **In the TUI** (D18) the application is the leader of a new session whose terminal is a pseudo-terminal: Tube holds the other side, reads what the application prints (§13.9), and writes what the user types on the Output tab. `TERM` is `dumb` for the application, so that programs print plain lines. The size of the pseudo-terminal is the size of the Output tab.

The application is found in `PATH` by Tube before anything else is done: a name that is not found ends Tube with status 127 before the TUI opens.

### 8.6 What the application shares with the machine

Everything but the network: the files, the home folder, the environment, the display (X11 and Wayland sockets are files), the sound, the D-Bus buses. It does not see the machine's network interfaces, its loopback (a service on the machine's `127.0.0.1` cannot be reached) or its abstract Unix sockets.

### 8.7 Programs that hand their work over

A program that, instead of working itself, asks a process that already runs to do it, is not in the VPN: that process is outside the namespace. A second `firefox` asks the first one to open a window; a terminal emulator asks its server. For Firefox and Thunderbird, Tube sets `MOZ_NO_REMOTE=1` (D15): with a Firefox already open on the same profile, `tube firefox` then refuses to start instead of opening a window outside the VPN, and `tube firefox -P <another profile>` starts a second Firefox inside it. `docs/GUIDE.md` says this for the user. Tube cannot know it for every program.

### 8.8 The check at the start

Before the TUI opens (and before `--profile` connects), Tube starts a keeper that only makes the namespace and the device, and ends. When that fails, Tube ends with an error that says what the machine does not allow and which setting changes it (`SandboxUserNamespaceDenied`, `SandboxTunUnavailable`; §3). It takes a few milliseconds.

## 9. The pump

Two goroutines: one reads IP packets from the TUN's file descriptor and writes each to the tunnel; the other reads from the tunnel and writes to the device. The buffers are as large as the MTU.

- A packet that the device refuses (`EINVAL`, `EIO`) or that the tunnel refuses for its size is dropped and counted; it does not end the pump.
- When the tunnel's `Read` or `Write` fails, the pump ends with that error (`io.EOF` when the server left).
- The counters (packets and bytes each way, and the dropped packets) can be read while it runs. `Stop` ends both goroutines and returns them; it closes neither the device, which lives across reconnects and switches, nor the tunnel.

## 10. The session

One run of an application: `session.Start` takes a connected tunnel, starts the keeper, configures the namespace, starts the pump and the application. The namespace and the application live as long as the session; the tunnel and its pump can be replaced while it lives.

- **Reconnecting (D14).** When the pump ends because the tunnel failed, the application keeps running with no network. Tube says so (a line, below) and dials the profile again, with the credentials of this run (kept in memory only), after 1, 2, 4, 8, 15 and then every 30 seconds. On success it sends `configure` with the new `Network`, starts a new pump and says so. When the server refuses the credentials, Tube stops trying and says so. A reconnect never opens another way to the network.
- **Disconnecting and switching (D17, D20).** `Disconnect` stops the pump and closes the tunnel, and Tube does not connect again by itself: the application keeps running with no network. `Use` gives the session another tunnel (of the same profile or of another one): the pump stops, the old tunnel is closed, the namespace is configured for the new one, and a new pump starts. When the namespace cannot be configured for the new tunnel, that tunnel is closed and the session stays disconnected. The profile that is dialed again after a loss is the one in use.
- **Status.** The session tells, at any moment: its state (connected, connecting again, disconnected, login refused), since when, the profile in use, the tunnel's addresses, the DNS servers in the namespace's `resolv.conf`, the packets and bytes that passed each way since the session started (over all its tunnels) and the packets that were dropped, and whether the application runs or with which status it ended. It calls back when its state changes.
- **End.** When the application has ended, the session stops the pump, closes the tunnel (the library tells the server), waits for the keeper and removes the temporary folder. The exit status is the application's, or 128 plus the number of the signal that ended it. Processes that the application left in the background keep running without a network.
- **Signals.** With `--profile`, while the application runs, Tube and the keeper catch SIGINT and SIGQUIT and do nothing with them (the terminal sends them to the application too). In the TUI the terminal sends none: the keys go through the pseudo-terminal (§8.5). In both ways, on SIGTERM or SIGHUP Tube passes the signal to the application, and SIGKILL five seconds later. A signal is caught, never ignored: an ignored signal would stay ignored in the application.
- **Tube's own failures** end it with status 125, after an error message on standard error; an application that cannot be executed is 126, one that is not found 127, a mistake in the arguments 2.
- **Lines of Tube** while the application runs (connected, the VPN ended, connected again, disconnected, now using another profile, the warning of §8.4) start with `tube: `. With `--profile` they go to standard error, and `--quiet` leaves them out. In the TUI they are the events of the Status tab (§13.8), and nothing is written to the terminal.

## 11. The command line

```
tube [options] [--] <application> [arguments...]    run the application through a VPN
tube [options]                                      only keep the profiles (the TUI)

  -p, --profile <name>    use this profile and do not show the TUI
      --auth-file <path>  with --profile: a file with the user name on the first line and
                          the password on the second
  -q, --quiet             with --profile: write no lines of Tube's own while the application runs
      --version           print the version
  -h, --help              print this help
```

- The options end at the first argument that is not one; everything after it is the application and its arguments, untouched.
- **Without `--profile`**, Tube opens the TUI, which needs a terminal on standard input and output; without one Tube ends with an error that names `--profile`. The user selects a profile; the application starts on a pseudo-terminal and the TUI stays open (§13.8) until the user quits. Tube then exits with the application's status. When the user quits before a profile was selected, the status is 130 (0 when there was no application).
- **With `--profile <name>`** there is no TUI: Tube connects, the application gets Tube's terminal (D19), and Tube exits with its status when it ends. The profile is the one with exactly that name, else the only one that has it when compared without case. With a profile that asks for a password and no `--auth-file`, Tube asks on the terminal (the user name when none is remembered, then the password, without echo); with no terminal it is an error. `--profile` with no application is a mistake in the arguments. The file of `--auth-file` must be readable by the user only.
- `tube --version` prints `tube <version>`. The version is `devVersion` of `internal/app/version.go`, or the one that the release build sets.

## 12. The application layer (`internal/app`)

`app.App` is what the TUI and `cmd/tube` use. It imports no UI library.

### 12.1 Snapshot

`App.Snapshot()` returns a value that the TUI draws from and never changes: the application to run (its name and arguments, or none), and the profiles of §6.4 (id, name, remotes, needs a password, remembered user name, DNS servers, last used, the ignored options, the names of the files, or the reason it is broken).

### 12.2 Commands and changes

The commands block and take a context; the TUI calls each inside a command of its own (a `tea.Cmd`), so its screen never waits. They are: add a profile (path, name, without scripts or not), rename, set the user name, set the DNS servers, begin and end an edit of a profile's text (the App gives the path of a copy for the editor, then reads it back), delete, and connect (profile, credentials, whether to remember the user name). Cancelling the context of a connect cancels it.

`App.Subscribe` registers a function that is called after every change: of the profiles, of the session's state, of the application's output. The calls are joined so that a program that prints fast causes at most about twenty a second.

### 12.3 How an error is shown

`app.Explain(err)` gives the sentence for the user: for the Tags that Tube knows (its own, and those of gopenvpn-conn for a wrong password, a server that does not answer, a certificate that is not accepted, an option that is not supported, a file that is missing) a sentence in plain words, with the option and the line when the error has them; for any other error its message. The Tag is shown after it, in parentheses. The whole error (route, metadata, stack) goes to the log. No sentence has a secret in it.

### 12.4 Running

- `App.Run(ctx)` is the way of `--profile`: it runs the application on Tube's terminal through the connected VPN and returns its exit status when it has ended.
- `App.Start(ctx)` is the way of the TUI: it starts the application on a pseudo-terminal and returns. From then on the App gives: the **run state** (what the session's status has, with the rates of the last second and Tube's last 100 lines as events), the **output** (the last 5000 lines that the application printed, as plain text that is safe to draw), `Input` (bytes for the application's terminal), `ResizeOutput`, `Disconnect`, `Switch` (connect another profile, and when it is connected, give it to the session in place of the one in use: D20), and `Stop` (SIGTERM, SIGKILL five seconds later). `App.Finish` stops the application when it still runs, waits for it, cleans up and returns its exit status; `cmd/tube` calls it after the TUI has ended.

## 13. The TUI

Bubble Tea, on the alternate screen, with the mouse (§13.12). One model; pages are values of it. A terminal smaller than 40 columns by 10 rows shows only "The terminal is too small".

### 13.1 Frame and colors

The first line: `Tube` and what will happen ("select a profile to run firefox", or "profiles"). The last line: the keys of the page. Above it, one line for a message (green) or an error (red), which the next key or click clears.

The TUI has its own colors (D21), made with Lip Gloss: the name `Tube` and the selected row on violet; the application and the keys in cyan; a server, an address and what was sent in blue; what is well in green (connected, in use, what was received, an `.ovpn` file); what needs attention in yellow (connecting, a password, a question); an error and "no network" in red; what matters less in gray. There is one set of colors for a dark terminal and one for a light one: Tube asks the terminal for its background color when it starts, and takes a dark one when the terminal does not answer. The colors are given as 24 bits, and Bubble Tea brings them down to what the terminal has.

**Color is only an addition.** What is selected also has `>` in front and is bold; the current tab is in brackets; a button is in brackets; an error starts with `Error:`; the profile in use has `*`. With `NO_COLOR`, or on a terminal without colors, what is selected is drawn reversed.

### 13.2 Profiles (the first page)

A list: name, server, `password` when the profile asks for one, last used. With no profile: "No profiles yet. Press a to add an .ovpn file."

| Key | What it does |
|---|---|
| `↑` `↓` `k` `j`, `Home` `End`, `PgUp` `PgDn` | move |
| `Enter` | with an application: connect and run it (§13.6). Without one: open the profile's page |
| `a` | add a profile (§13.3) |
| `e` | open the profile's page (§13.4) |
| `d` | delete, after a question |
| `?` | the help page |
| `q`, `Ctrl-C` | quit (status 0 without an application; 130 with one, as nothing ran) |
| a click on a row | select it; a double click is `Enter` |
| the wheel | scroll the list |

The profile that was used last is selected when the page opens.

### 13.3 Add

From the top: **the list of a folder** (D23), the field **File** (the path of the `.ovpn` file; `~` is the home folder), the field **Name**, the error of the last add, and the buttons `[ Add ]`, `[ Cancel ]` and the mark `[ ] all files`. The question of §6.2 about scripts appears here. `Esc` goes back.

**The list of a folder.** A box with the path of the folder on its upper edge, and in it: `../` (the folder above; not in `/`), then the folders, then the files, each sorted by name without case. Only the files whose name ends in `.ovpn` (any case) are listed, and no name that begins with a dot; with `all files` on, every name is. A link to a folder is a folder. A name that is not safe to draw (§6.5) is never listed. The list opens on the working folder of Tube, and, after an add, on the folder of that add. At most 10,000 names of a folder are read and 2,000 are listed; the lower edge then says so, and it says which rows are shown when not all fit. A folder that cannot be read says so; an empty one too. A terminal that is too short for a list of two rows shows the page without the list (and without `all files`).

| In the list | What it does |
|---|---|
| `↑` `↓`, `PgUp` `PgDn`, `Home` `End`, the wheel | move |
| `Enter`, a double click | on a folder: open it. On a file: write it in the File field and go to the Name field |
| `→` | open the folder |
| `Backspace`, `←` | the folder above, with the folder that was shown selected |
| a click on a row | select it; a file is also written in the File field |
| a click on a folder of the path on the upper edge | open that folder |
| `Tab` | go to the File field (`Shift-Tab`: to `all files`) |
| text | goes to the File field, which gets the focus; `/` and `~` start a new path there |

**The fields.** The list follows the File field: it shows the folder of the path that is typed or pasted there, with the first name selected that begins like the last part of the path; and a folder that is opened in the list is written in the field. `Tab` in the File field completes the name of a file or folder as far as the names of its folder allow. `↑` and `↓` go from one part of the page to the next (list, File, Name, `all files`), `Tab` too outside the File field, and `Shift-Tab` back. A click in a field gives it the focus and puts the cursor there. The Name field shows, dim, the name that the profile gets when none is typed. `Space` on `all files`, or a click, changes the mark. `Enter` in a field or on `all files`, or a click on `[ Add ]`, adds; a click on `[ Cancel ]` is `Esc`. An error stays on the page, under the fields, until the next key or click. While an add runs, the page takes no key and no click.

### 13.4 The profile's page

Shows: name, every remote, whether it asks for a password, the remembered user name, the DNS servers ("from the VPN" when none is set), the files, the options that are ignored. Keys: `r` rename, `u` user name, `n` DNS servers (each opens one text field on the page; `Enter` saves, `Esc` cancels; a click in the field puts the cursor there), `t` edit the text, `d` delete, `Enter` connect and run (with an application), `Esc` back.

### 13.5 The editor

`t` runs the user's editor on a copy of the profile (§6.3): `$VISUAL`, else `$EDITOR`, else the first of `nano`, `vi` that is in `PATH`; the value is split at spaces into a program and its arguments, and is never given to a shell. Bubble Tea gives the terminal to the editor and takes it back. With no editor found, a message says to set `EDITOR`.

### 13.6 Login and connecting

- A profile that asks for a password opens the login page first: user name (filled with the remembered one), password (drawn as nothing: no characters, no dots), and "remember the user name" (on by default). `Enter` connects, `Esc` goes back. With the mouse: a click on a field gives it the focus (and puts the cursor of the user name there; the password field shows nothing, so a click moves nothing in it), a click on "remember" changes it, and the buttons `[ Connect ]` (`[ Use ]` when the page was opened from the Network tab) and `[ Cancel ]` are `Enter` and `Esc`.
- The connecting page shows the profile's name and the seconds that have passed. `Esc`, or a click on `[ Cancel ]`, cancels the connecting and goes back.
- When it fails: the error (§12.3) on the profiles page; a wrong password goes back to the login page with the user name kept.
- When it succeeds: the time of use is saved, the application starts, and the run screen opens (§13.8).

### 13.7 Help

`?` shows the keys of every page and tab, what the mouse does, and three lines about what Tube does and §8.7. Any key or click closes it. (On the Output tab `?` goes to the application, as every key does there.)

### 13.8 The run screen and the Status tab

While the application runs, and after it has ended until the user quits, the TUI shows the run screen:

- The first line: the application, the profile in use, the state (`connected` and for how long, `connecting again…`, `disconnected`, `login refused`), and the rates down and up. After the application ended: that it ended, and its status.
- The second line: the tabs, `Status`, `Output`, `Network`; the current one is in brackets. `Tab` goes to the next tab and `Shift-Tab` to the one before, on every tab; a click on a tab shows it.
- The **Status** tab (the first one shown): the application and whether it runs; the profile and the state; the tunnel's addresses, the DNS servers in use and the MTU; for each direction the bytes and packets since the start and the rate of the last second; the dropped packets; and the events, which are Tube's lines (§10) with their time, the newest last.
- Sizes are written with decimal units (`1.5 MB`, `80.0 kB/s`), times as `hh:mm:ss`.

### 13.9 The Output tab

- It shows the last lines that the application printed, cut to the width. It follows the end; `PgUp` and `PgDn` scroll, the wheel scrolls three lines, `Home` goes to the oldest line that is kept (5000 are), `End` follows again (scrolling down to the end does too).
- What the application writes is made into plain lines first (`internal/core/termlog`): a new line ends a line; a carriage return alone starts the line again (a progress bar is one line); a backspace takes a character away; a tab becomes spaces; escape sequences (colors, cursor moves, titles) and every other control or format character are dropped; a line is at most 1000 characters. Nothing that the application prints can move the TUI's cursor or change its colors.
- **Every other key typed on this tab goes to the application**, as bytes written to its terminal: text as it is, Enter as a carriage return, Backspace, Ctrl-C, Ctrl-D, Ctrl-U, Escape; pasted text too. So Ctrl-C on this tab stops `ping`, and a program that asks a question can be answered. The mouse never goes to the application: a click on the output does nothing. The terminal is a dumb one (D18): a program that wants the whole screen is run with `--profile`.
- After the application ended, the keys go nowhere, and `q` quits.

### 13.10 The Network tab

- The list of the profiles, with the one in use marked, and above it one line that says what the application has now: the profile it uses, or that it has no network and why (connecting again, disconnected, login refused).
- `Enter` on a profile, or a double click on it, switches to it (D20): a profile that asks for a password opens the login page first; while the new profile connects, the one in use stays in use, and `Esc` cancels. When it is connected, it replaces the one in use. When it fails, the error is shown and nothing changes. When the application has no network, `Enter` on a profile (also the one last used) connects it.
- `x` disconnects, after a question: the application keeps running without network, and Tube does not connect again until a profile is chosen.
- Adding, changing and deleting profiles is not done here: it is done before a profile is selected (§13.2 to §13.5).

### 13.11 Quitting

- While the application runs, `q` or `Ctrl-C` on the Status or the Network tab asks: "<application> is still running. Stop it and quit?" On yes, Tube stops the application (§12.4) and quits when it has ended; a second `Ctrl-C` quits at once.
- After the application ended by itself, a message says so with its status, and `q` quits without a question.
- Tube exits with the application's exit status.

### 13.12 The mouse

The TUI asks the terminal for the clicks and the wheel of the mouse (D22). Everything can still be done with the keys alone.

- **A click of the left button** does what is under it: a row of a list is selected; a tab is shown; a button (`[ Add ]`, `[y] Delete`, ...) is pressed; a field gets the focus and its cursor; a mark (`[x]`) changes; and **a key on the last line is pressed**, by a click on the key or on the word next to it, so every key that the page names can be clicked. The other buttons of the mouse do nothing.
- **A double click** (two clicks within half a second, on the same line and at most two cells apart) on a row is `Enter` on that row: run or open a profile, use it on the Network tab, open a folder or choose a file on the add page. One click never connects, runs or adds anything: a click that only gives the terminal's window the focus must not start an application.
- **The wheel** scrolls the list of the page (the profiles, the folder, the Network tab) three rows, the selection staying inside what is shown, and the Output tab three lines.
- While a question is open, only its two answers and the keys of the last line can be clicked, and the wheel does nothing.
- What is clicked is always what is drawn: the page draws lines that know their parts that can be clicked, and a click is looked up in the same lines.
- While the TUI has the mouse, the terminal does not select text with it. Terminals select text while `Shift` is held; the help page says so.

## 14. Security

What Tube promises, and where it is kept:

| Promise | How | Tested by |
|---|---|---|
| The application's packets leave only through the tunnel | a network namespace with the loopback and `tube0` only (§8.1 to §8.3) | a test lists the interfaces and routes inside, and one sees every packet of the application on the TUN |
| No network without the VPN | the device exists only while Tube holds it; the pump is the only thing that moves packets; no fallback (§7, §8.2, §10) | tests that a connection inside fails before the pump runs, after the tunnel ended, and after Tube was killed |
| The names go to the VPN's DNS | §8.4 | a test resolves a name inside and sees the query on the TUN, to the configured server; with no server, the lookup fails |
| The application cannot change the namespace's network | it runs in a nested user namespace with no capability over it (§8.5) | a test tries (`RTM_NEWROUTE`) from inside and is refused |
| No root | nothing needs it; archtest rule 5 keeps `sudo` and helpers out | |
| Keys and passwords stay private | modes 0700 and 0600 (§5); the password only in memory (D12); never in a log, an error or on the screen | tests of the modes; tests that a known secret appears in no log line and no error |
| Hostile text cannot reach the terminal | `sanitize.Display` (§6.5); search domains checked (§7) | tests with escape sequences in names, remotes and pushed options |
| A profile runs nothing | only `ovpn` parses it; script options are refused or disabled (§6.2) | a test adds a profile with `up` and sees that nothing ran |
| The application's output cannot reach the user's terminal as it is | it is read from a pseudo-terminal and made into plain lines (§13.9) | a test writes random bytes and escape sequences and finds no control character in the lines |
| A switch or a disconnect opens no other way | they replace or remove the tunnel behind the same device; the namespace does not change (§10) | tests that a connection inside fails after a disconnect, and during a switch that fails |

Not promised: hiding from the VPN provider; protection against an application that talks to a process outside the namespace (§8.7, §17); anything about the traffic of the rest of the machine.

## 15. Logging

`slog`, JSON lines, to `tube.log` in the state folder (mode 0600). When the file passes 1 MiB it becomes `tube.log.1` (replacing the old one). The log has the start and the end of each run, the profile's id and name, the remote that answered, the addresses and DNS servers of the tunnel, the counters of the pump, and every error with its Tag and route. It never has a key, a password, a token, or a line of a profile. Nothing is logged to the terminal.

## 16. Testing

### 16.1 Isolation

No test, proof or check uses the user's profiles, state or applications. Go tests call `storagetest.Isolate(t)`, which sets `TUBE_CONFIG_DIR`, `TUBE_STATE_DIR`, `HOME` and the XDG variables to folders under `t.TempDir()`. Scripts run the program through `scripts/isolated.sh`. No test connects to a server on the internet, and none starts `firefox` or another application of the user.

### 16.2 Tests of the namespace

They run in the ordinary `go test`, as the user, and skip (saying why) when the machine does not allow user namespaces or the TUN device. The test is Tube's side: it starts the keeper from the test binary, holds the TUN's file descriptor, and plays the VPN itself: it reads the packets that a command inside sends (`ping`, a small program that the test builds) and answers them (an ICMP echo reply, a DNS answer). This shows §8 and the refusals of §14 without a VPN server.

### 16.3 Tests against the real OpenVPN server

Build tag `tube_server`, run under `scripts/vpn-ns.sh` (a private user and network namespace in which the test is root and has a loopback of its own). `vpntest` makes a CA, a server certificate and a client certificate in Go, writes a server configuration and a client profile, and starts `openvpn` as a server on `127.0.0.1` of that namespace, in its own process group, stopped by `t.Cleanup`. The tests skip when `openvpn` is not installed. They show the whole path: `tube --profile test ping -c 2 <the server's tunnel address>` ends with status 0; a name is resolved by a DNS server that the test runs on the server's tunnel address; when the server is stopped the application has no network, and when it is started again, it has (D14).

### 16.4 Tests of the TUI

They drive the model directly: `Update` with key, mouse and window-size messages, then `View`, compared with golden text files, with a fake clock, a fixed color profile and the fake application of `apptest`. The golden files hold the text without its colors. The add page of these tests lists folders that are held in memory, not the folders of the machine; the tests of the real list read folders under `t.TempDir()`. No terminal is needed.

### 16.5 Proofs

A step that says so makes a proof with `scripts/tui-proof.sh`: the real program, isolated, in a private tmux server, gets keys and clicks of the mouse and the screen is saved as text in `docs/implemented-proof-images/`. A proof is looked at, not compared.

## 17. Known limits

- A program that hands its work to a process outside the namespace is not in the VPN (§8.7). The same holds for a program that asks a daemon of the machine over D-Bus or a Unix socket to do network work for it (a download service, a resolver asked over D-Bus by a program that does not use the C library's lookup).
- Services on the machine's own `127.0.0.1` cannot be reached from the application (a local proxy, a local web server).
- Programs that need to gain rights (`sudo`, a setuid program) do not gain them inside the namespace.
- IPv6 works only when the VPN gives an IPv6 address; else the application has IPv4 only.
- What gopenvpn-conn does not support is not supported: compression, proxies, static keys, encrypted private keys, challenge/response logins (its README lists them). The parser's error names the option.
- In the TUI the application's terminal is a dumb one that shows plain lines: a program that draws the whole screen (`vim`, `htop`, `less`) or needs every key (`ssh` to a shell with completion) is run with `--profile`, which gives it the real terminal. With `--profile` there are no tabs, and the profile cannot be switched while the application runs.
- While the TUI is open, the mouse belongs to it: the terminal selects text only while `Shift` is held (§13.12).
- Flatpak and Snap applications are not tested.

## 18. Milestones

| Milestone | Version | What works at its end |
|---|---|---|
| M0 Foundations | v0.1.0 | The module, the checks, the architecture tests, storage, clock, sanitize, logging |
| M1 Profiles and the VPN | v0.2.0 | The store of profiles; connecting a stored profile to a real OpenVPN server in a test |
| M2 The namespace | v0.3.0 | The keeper, the TUN device, addresses, routes, DNS, the application, the pump, the session; the refusals of §14 are tested |
| M3 The command line | v0.4.0 | `tube --profile <name> <application>` works end to end against a real server |
| M4 A session that changes | v0.5.0 | Reconnecting (D14); disconnecting and switching (D17); the session's status; the log of a terminal |
| M5 The TUI: the profiles | v0.6.0 | `tube <application>`: the pages of §13.1 to §13.7; selecting a profile connects and runs the application |
| M6 The TUI: the running application | v0.7.0 | The run screen: the Status, Output and Network tabs (D16, D17, D18) |
| M7 Release | v0.8.0 | `README.md`, `docs/GUIDE.md`, `docs/SECURITY.md`, the release build, the manual checks listed for the user |
