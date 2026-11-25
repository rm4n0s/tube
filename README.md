# Tube

Tube runs one application through a VPN, and only that application. Every other program on the
machine keeps using the machine's own network.

```
tube firefox
tube --profile work curl https://example.com
```

`tube firefox` opens a terminal UI, in color, that you use with the keys or with the mouse. It
has your OpenVPN profiles (`.ovpn` files): you add them (from a list of your folders), look at,
change and delete them there, and select one. Tube connects to it and starts Firefox so that all
of Firefox's network goes through that VPN: TCP, UDP, ICMP and the looking up of names. The
terminal UI stays open while Firefox runs. It has three tabs:

- **Status**: the connection and its traffic.
- **Output**: what the application prints.
- **Network**: switch to another profile, or take the network away.

Before the VPN is connected, and after it has ended, the application has no network at all.

`tube --profile work curl https://example.com` does the same without the terminal UI. It is for
scripts and for programs that need the whole terminal.

```
 Tube   ping through test   connected 00:00:00   ↓ 0 B/s  ↑ 0 B/s
[Status]  Output  Network
────────────────────────────────────────────────────────────────────────────────────────────────────
Application   ping, running
VPN           test, connected for 00:00:00
Address       10.8.0.2/24
DNS servers   -
MTU           1500

↓ Received    168 B    2 packets   0 B/s
↑ Sent        168 B    2 packets   0 B/s
  Dropped     0 packets
```

(The first lines of `docs/implemented-proof-images/ui-status.txt`, made with `scripts/tui-proof.sh`
against a test server.)

## How it works

Tube needs no root, no `sudo`, no system service and no installed `openvpn`. The application
runs in a private user, network and mount namespace. Its only way out is a TUN device, and Tube
feeds that device from the VPN connection. Nothing of the machine's network configuration
changes.

## What it needs

- Linux, on x86-64 or arm64.
- A kernel that lets an ordinary user make user namespaces. Three settings decide it:
  - `user.max_user_namespaces` must be above 0.
  - On Debian, `kernel.unprivileged_userns_clone` must be `1`.
  - On Ubuntu 24.04 and later, `kernel.apparmor_restrict_unprivileged_userns` must be `0`, or
    an AppArmor profile must allow Tube.
- `/dev/net/tun` that you may open (mode 0666, the default of the distributions).
- A terminal, for the terminal UI. `--profile` needs none.

Tube checks this when it starts, and says what is missing. `docs/GUIDE.md` ("What Tube needs")
shows how to look at these settings.

## Building and installing

With Go 1.27.3 or later:

```
go install github.com/rm4n0s/tube/cmd/tube@latest
```

or, in a clone of this repository:

```
go build -o tube ./cmd/tube
```

Tube has no cgo: `CGO_ENABLED=0` builds it too. `scripts/build-release.sh` builds the archives of a
release for linux/amd64 and linux/arm64.

## Limits

- A program that hands its work to a process outside the namespace is not in the VPN. For
  example, a second `firefox` asks the Firefox that is already open to open a window.
  `docs/GUIDE.md` says what to do.
- Services on the machine's own `127.0.0.1` cannot be reached from the application.
- Programs that need to gain rights (`sudo`, a setuid program) do not gain them inside.
- IPv6 works only when the VPN gives an IPv6 address.
- What gopenvpn-conn does not support is not supported: compression, proxies, static keys,
  encrypted private keys, and challenge/response logins.
- In the terminal UI the application's terminal shows plain lines. A full-screen program
  (`vim`, `htop`, `ssh`) is run with `--profile`.
- Flatpak and Snap applications are not tested.

## Documents

- `docs/GUIDE.md`: how to use Tube, and what its errors mean.
- `docs/SECURITY.md`: what Tube promises, what it does not, and how to report a problem.
- `docs/SPEC.md`: the specification.
- `docs/IMPLEMENTATION.md` and `docs/IMPLEMENTED.md`: the plan, and the log of what was done.
- `docs/MANUAL-CHECKS.md`: the checks that need a real VPN provider or a real desktop.

## License

GPL-2.0-only. See `LICENSE`.

Tube connects to OpenVPN servers with [gopenvpn-conn](https://github.com/rm4n0s/gopenvpn-conn).
It is not OpenVPN's own program and is not affiliated with OpenVPN Inc; OpenVPN is a trademark
of OpenVPN Inc.
