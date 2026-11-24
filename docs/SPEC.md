# Tube: Specification

Tube runs one application through a VPN, and only that application. This file is the source of truth for what Tube does. `docs/IMPLEMENTATION.md` breaks it into steps.

## 1. Purpose

- `tube firefox` or `tube ping -c 3 example.com` opens a terminal UI (TUI) with the user's **profiles** (OpenVPN client profiles, `.ovpn`). The user adds, looks at, changes and deletes profiles there, and selects one. Tube connects to it and starts the application.
- **All the network of the application goes through the VPN**: TCP, UDP, ICMP and the looking up of names. Nothing else on the machine changes: other programs keep using the machine's network.
- **Nothing leaks.** The application has no way to the network but the tunnel. Before the VPN is connected and after it has ended, the application has no network at all.
- Tube needs no root, no `sudo`, no system service and no installed `openvpn`.
- When the application ends, Tube disconnects and exits with the application's exit status.

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
| D13 | `--profile <name>` runs the application with a profile without showing the TUI | scripts and the tests against the real server need a way without a terminal UI |
| D14 | When the VPN connection ends while the application runs, Tube connects again by itself; the application has no network in between | a browser session outlives a short loss of the link; D2's "nothing else" still holds |
| D15 | Tube sets `MOZ_NO_REMOTE=1` for the application | without it `tube firefox`, with a Firefox already open, hands the page to the open one, which is outside the VPN (§8.7) |

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
| `cmd/tube` | `main`: the keeper's entry (§8.1), the flags (§11), then the TUI or `--profile`, then the session |
| `internal/app` | The application behind the TUI: owns the store, gives snapshots, commands and events (§12), connects, and runs the session. Also `version.go` and the sentences that errors are shown with |
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
| `internal/core/session` | One run: a tunnel, a namespace, a pump and the application, with reconnecting (§10) |
| `internal/archtest` | Tests of the rules of §4.2, and the error-tag tool (`cmd/errortags`) |

### 4.2 Rules that `internal/archtest` checks

1. `internal/core/...` and `internal/app` import no UI library (`charm.land/...`, `github.com/charmbracelet/...`) and not `internal/tui`.
2. `internal/tui` imports `internal/app` and, of the core, only `clock` and `sanitize`. Nothing imports `internal/tui` but `cmd/tube`.
3. Only `internal/core/vpn` and `internal/core/profiles` import `github.com/rm4n0s/gopenvpn-conn/...`; nothing imports its `vpnnet`.
4. Nothing imports the standard library `errors`, and nothing calls `fmt.Errorf` (CLAUDE.md "Error handling").
5. Nothing imports `os/exec` but `internal/core/sandbox` (which starts the keeper and the application), `internal/tui` (the editor, §13.5) and test files. Tube never runs `ip`, `unshare`, `openvpn` or a shell.
6. Every Go file starts with `// SPDX-License-Identifier: GPL-2.0-only`.
7. Every error Tag (the first argument of `errors.New`, `errors.NewErr`, `errors.NewErrf`, a string literal) is asserted by a test, by its route (`HasRoute("...Tag")`). `scripts/error-tags.sh --untested` lists the ones that are not; `docs/ERROR-TAGS-UNTESTED.md` is the only list of exceptions, each with its reason.

## 5. Files and folders

| Folder | Where | What is in it |
|---|---|---|
| Configuration | `$TUBE_CONFIG_DIR`, else `$XDG_CONFIG_HOME/tube`, else `~/.config/tube` | `profiles/<id>/meta.json`, `profiles/<id>/files/...` |
| State | `$TUBE_STATE_DIR`, else `$XDG_STATE_HOME/tube`, else `~/.local/state/tube` | `tube.log`, `tube.log.1` |
| Temporary | a new folder under `$XDG_RUNTIME_DIR`, else under `os.TempDir()`, one per run, removed at the end | what the namespace needs (§8.4) |

- Every folder Tube makes has mode 0700 and every file mode 0600, whatever the umask. A folder that exists with wider rights is narrowed when Tube first writes into it.
- A file is written atomically: to a temporary file in the same folder, `fsync`, rename.
- There is no lock: several Tubes may run at once, each with its own VPN connection and namespace. The store reads the folder each time it lists; when two Tubes change the same profile, the last write wins.
- A relative path in one of the variables is an error (`StorageDirNotAbsolute`).

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

A profile, and everything a VPN server sends, is hostile text. `sanitize.Display(s, max)` drops control characters, escape sequences, bidirectional controls and invalid UTF-8, turns every white space into a space, and cuts to `max` runes with `…`. Everything from a profile or a server passes through it before it is stored in `meta.json`, logged or drawn. A secret is never drawn, also not sanitized: the TUI shows of a profile only what §6.4 lists.

## 7. The VPN connection

`vpn.Dial(ctx, store folder of the profile, credentials)` parses the stored profile (as §6.2 step 1), sets `Config.Credentials` when the profile needs them, and calls `Profile.Dial`. It returns a `vpn.Tunnel`:

```go
type Tunnel interface {
    Read(packet []byte) (int, error)   // one IP packet from the VPN
    Write(packet []byte) (int, error)  // one IP packet to the VPN
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
| Tube | `run` (argv) | start the application |
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

On `run` the keeper starts the application in a nested user namespace (`CLONE_NEWUSER`) in which the user's own uid and gid are mapped back: the application sees itself as the user, owns its files as always, and has no capability over the network namespace, so it cannot change the addresses, the routes or the device. It gets the keeper's terminal, working folder and environment, with `TUBE=1`, `TUBE_PROFILE=<name>` and `MOZ_NO_REMOTE=1` added (D15). It stays in Tube's process group, so the keys of the terminal (Ctrl-C, Ctrl-\, Ctrl-Z) reach it as they would without Tube.

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
- `Stop` ends both goroutines and returns the counters (packets and bytes each way); it does not close the device, which lives across reconnects.

## 10. The session

One run of an application: `session.Start` takes a connected tunnel, starts the keeper, configures the namespace, starts the pump and the application; `Wait` returns when the application has ended.

- **End.** When the application ends, Tube stops the pump, closes the tunnel (the library tells the server), waits for the keeper, removes the temporary folder and exits with the application's exit status, or 128 plus the number of the signal that ended it. Processes that the application left in the background keep running without a network.
- **Signals.** While the application runs, Tube and the keeper ignore SIGINT and SIGQUIT (the terminal sends them to the application too). On SIGTERM or SIGHUP, Tube passes the signal to the application, and SIGKILL five seconds later.
- **Tube's own failures** end it with status 125, after an error message on standard error; an application that cannot be executed is 126, one that is not found 127, a mistake in the arguments 2.
- **Reconnecting (D14).** When the pump ends because the tunnel failed, the application keeps running with no network. Tube writes a line to standard error and dials the profile again, with the credentials of this run (kept in memory only), after 1, 2, 4, 8, 15 and then every 30 seconds. On success it sends `configure` with the new `Network`, starts the pump and writes a line. When the server refuses the credentials, Tube stops trying and says so. A reconnect never opens another way to the network.
- **Lines of Tube** on standard error while the application runs (connected, the VPN ended, connected again, the warning of §8.4) start with `tube: `, and `--quiet` leaves them out.

## 11. The command line

```
tube [options] [--] <application> [arguments...]    run the application through a VPN
tube [options]                                      only keep the profiles (the TUI)

  -p, --profile <name>    use this profile and do not show the TUI
      --auth-file <path>  with --profile: a file with the user name on the first line and
                          the password on the second
  -q, --quiet             write no lines of Tube's own while the application runs
      --version           print the version
  -h, --help              print this help
```

- The options end at the first argument that is not one; everything after it is the application and its arguments, untouched.
- Without `--profile`, Tube opens the TUI. It needs a terminal on standard input and output; without one it ends with an error that names `--profile`.
- `--profile <name>` takes the profile with exactly that name, else the only one that has it when compared without case. With a profile that asks for a password and no `--auth-file`, Tube asks on the terminal (the user name when none is remembered, then the password, without echo); with no terminal it is an error. `--profile` with no application is a mistake in the arguments.
- `tube --version` prints `tube <version>`. The version is `devVersion` of `internal/app/version.go`, or the one that the release build sets.

## 12. The application layer (`internal/app`)

`app.App` is what the TUI and `cmd/tube` use. It imports no UI library.

### 12.1 Snapshot

`App.Snapshot()` returns a value that the TUI draws from and never changes: the application to run (its name and arguments, or none), and the profiles of §6.4 (id, name, remotes, needs a password, remembered user name, DNS servers, last used, the ignored options, the names of the files, or the reason it is broken).

### 12.2 Commands and events

Commands that can take time (reading files, connecting) run in a goroutine of the App and answer with an event; the TUI turns events into `tea.Msg`. The commands: add a profile (path, name, without scripts or not), rename, set the user name, set the DNS servers, read the text of a profile for the editor and replace it, delete, connect (profile, credentials), cancel the connecting. The events: the profiles changed, a command failed (the error), connecting started, connected (the session is ready to run), connecting failed.

`App.Run(ctx, stdio)` runs the session that `connect` made, after the TUI has ended and given the terminal back, and returns the exit status.

### 12.3 How an error is shown

`app.Explain(err)` gives the sentence for the user: for the Tags that Tube knows (its own, and those of gopenvpn-conn for a wrong password, a server that does not answer, a certificate that is not accepted, an option that is not supported, a file that is missing) a sentence in plain words, with the option and the line when the error has them; for any other error its message. The Tag is shown after it, in parentheses. The whole error (route, metadata, stack) goes to the log. No sentence has a secret in it.

## 13. The TUI

Bubble Tea, on the alternate screen. One model; pages are values of it. Colors come from Lip Gloss and follow `NO_COLOR`. A terminal smaller than 40 columns by 10 rows shows only "The terminal is too small".

### 13.1 Frame

The first line: `Tube` and what will happen ("select a profile to run firefox", or "profiles"). The last line: the keys of the page. Above it, one line for a message or an error (red), which the next key clears.

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

The profile that was used last is selected when the page opens.

### 13.3 Add

Two fields: the path of the `.ovpn` file (`~` is the home folder; `Tab` completes the name of a file or folder) and the name. `Enter` adds, `Esc` goes back. An error stays on the page, under the fields. The question of §6.2 about scripts appears here.

### 13.4 The profile's page

Shows: name, every remote, whether it asks for a password, the remembered user name, the DNS servers ("from the VPN" when none is set), the files, the options that are ignored. Keys: `r` rename, `u` user name, `n` DNS servers (each opens one text field on the page; `Enter` saves, `Esc` cancels), `t` edit the text, `d` delete, `Enter` connect and run (with an application), `Esc` back.

### 13.5 The editor

`t` runs the user's editor on a copy of the profile (§6.3): `$VISUAL`, else `$EDITOR`, else the first of `nano`, `vi` that is in `PATH`; the value is split at spaces into a program and its arguments, and is never given to a shell. Bubble Tea gives the terminal to the editor and takes it back. With no editor found, a message says to set `EDITOR`.

### 13.6 Login and connecting

- A profile that asks for a password opens the login page first: user name (filled with the remembered one), password (drawn as nothing: no characters, no dots), and "remember the user name" (on by default). `Enter` connects, `Esc` goes back.
- The connecting page shows the profile's name and the seconds that have passed. `Esc` cancels the connecting and goes back.
- When it fails: the error (§12.3) on the profiles page; a wrong password goes back to the login page with the user name kept.
- When it succeeds: the time of use is saved, the TUI ends, the terminal is given back, and the application starts (§10).

### 13.7 Help

`?` shows the keys of every page and three lines about what Tube does and §8.7. Any key closes it.

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

They drive the model directly: `Update` with key and window-size messages, then `View`, compared with golden text files, with a fake clock, a fixed color profile and the fake application of `apptest`. No terminal is needed.

### 16.5 Proofs

A step that says so makes a proof with `scripts/tui-proof.sh`: the real program, isolated, in a private tmux server, gets keys and the screen is saved as text in `docs/implemented-proof-images/`. A proof is looked at, not compared.

## 17. Known limits

- A program that hands its work to a process outside the namespace is not in the VPN (§8.7). The same holds for a program that asks a daemon of the machine over D-Bus or a Unix socket to do network work for it (a download service, a resolver asked over D-Bus by a program that does not use the C library's lookup).
- Services on the machine's own `127.0.0.1` cannot be reached from the application (a local proxy, a local web server).
- Programs that need to gain rights (`sudo`, a setuid program) do not gain them inside the namespace.
- IPv6 works only when the VPN gives an IPv6 address; else the application has IPv4 only.
- What gopenvpn-conn does not support is not supported: compression, proxies, static keys, encrypted private keys, challenge/response logins (its README lists them). The parser's error names the option.
- Flatpak and Snap applications are not tested.

## 18. Milestones

| Milestone | Version | What works at its end |
|---|---|---|
| M0 Foundations | v0.1.0 | The module, the checks, the architecture tests, storage, clock, sanitize, logging |
| M1 Profiles and the VPN | v0.2.0 | The store of profiles; connecting a stored profile to a real OpenVPN server in a test |
| M2 The namespace | v0.3.0 | The keeper, the TUN device, addresses, routes, DNS, the application, the pump, the session; the refusals of §14 are tested |
| M3 The command line | v0.4.0 | `tube --profile <name> <application>` works end to end against a real server |
| M4 The TUI | v0.5.0 | `tube <application>`: the pages of §13, connect and run |
| M5 Reconnecting | v0.6.0 | D14 |
| M6 Release | v0.7.0 | `README.md`, `docs/GUIDE.md`, `docs/SECURITY.md`, the release build, the manual checks listed for the user |
