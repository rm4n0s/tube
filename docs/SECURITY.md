# Tube: security

Tube exists so that one application's traffic goes through a VPN and nowhere else. Tube also
keeps profiles that hold private keys. This file says what Tube promises, how it keeps each
promise, what it does not promise, and how to report a problem.

## What Tube promises

**The application's packets leave only through the VPN.** The application runs in a private
network namespace. That namespace has two interfaces: its own loopback, and the device `tube0`,
whose packets Tube moves to and from the VPN connection. It has no other interface and no route
but into the tunnel. A test lists the interfaces and routes inside, and another sees every packet
of the application on `tube0`.

**No network without the VPN.** The device `tube0` exists only while Tube holds it. Tube's pump
is the only thing that moves its packets. When the VPN fails, Tube never falls back to the
machine's network, and never starts the application outside the namespace. Before the VPN is
connected, after it ended, after you disconnected, and while another profile replaces it, the
application has no network at all. Tests show that a connection from inside fails before the
pump runs, after the tunnel ended, after a disconnect, during a switch that fails, and after Tube
was killed.

**The names go to the VPN's DNS.** Inside the namespace, `/etc/resolv.conf` names the DNS servers
of the VPN (or the ones that you set for the profile). The folders through which the C library
reaches the machine's own resolver (`systemd-resolved`, `nscd`, `avahi`) are covered with empty
ones. A test resolves a name inside and sees the query on `tube0`, to the configured server. With
no DNS server, the lookup fails.

**The application cannot change the network of its namespace.** It runs as you, in a nested user
namespace, with no capability over the namespace's network. It cannot add a route, an address or
a device. A test tries from inside, and is refused.

**No root.** Tube never uses `sudo`, a setuid helper or a capability on its binary. It changes
nothing of the machine: no device, address, route, firewall rule or DNS setting outside its
private namespace.

**Keys and passwords stay private.** Profiles and every file that holds a key are written with
mode 0600, in folders of mode 0700. A password is never written to a log, an error, or the
screen (the login page draws stars). It is kept in memory, and, **when you let Tube remember it,
in a file of mode 0600 in the profile's folder, not encrypted**: it is as private as the
profile's private key beside it, and who can read your files can read both. Turn "remember the
password" off on the login page, or forget it on the profile's page, to have none on disk. Tests check the modes, and check that a known secret
appears in no log line and no error.

**One way of giving a password is not private: `--password`.** An option is part of Tube's
command line, and while Tube runs, the other users of the machine can read it in the list of
processes (`ps`); a shell may also keep it in its history. Tube itself puts it in no log and no
error. On a machine that others use, give the password in a file of mode 0600 (`--auth-file`),
or let Tube ask on the terminal and save it: neither is on a command line.

**Hostile text cannot reach your terminal.** Everything that comes from a profile or a VPN server
(names, servers, pushed options, messages) is made safe before it is stored, logged or drawn:
control characters and escape sequences are replaced. What the application prints is read from a
pseudo-terminal and turned into plain lines. Tests put escape sequences into names, servers,
pushed options and the application's output, and find no control character on the screen.

**A profile runs nothing.** A profile is only parsed. Tube never runs a program that a profile
names (`up`, `down`, `script-security`, ...), and never reads a file that a profile names outside
the folder that it was added from. A test adds a profile with `up` and sees that nothing ran.

**A switch or a disconnect opens no other way.** A switch and a disconnect replace or remove the
VPN connection behind the same device; the namespace does not change.

## What Tube does not promise

- **Hiding from the VPN provider.** The provider sees the application's traffic, as with any VPN.
- **Programs that hand their work to a process outside the namespace.** A second `firefox`, a
  terminal emulator's client, or a program that asks a daemon of the machine (over D-Bus or a
  Unix socket) to do network work for it: that work is done outside the VPN. `docs/GUIDE.md`
  ("Firefox and other programs that are already open") says what Tube does for Firefox: it runs
  on a Firefox profile of Tube's own, so that it is never handed to a Firefox that is open. A
  Firefox profile that you name yourself (`tube firefox -P work`) is not protected that way.
- **The rest of the machine.** Every other program keeps using the machine's own network.
- **Services on the machine's own `127.0.0.1`.** The application cannot reach them, and nothing
  of them is protected or exposed by Tube.
- **Gaining rights inside.** `sudo` and setuid programs do not gain rights inside the namespace.
- **IPv6 without an IPv6 address from the VPN.** The application then has IPv4 only; IPv6 has no
  route, and does not leak.
- **What gopenvpn-conn does not support:** compression, proxies, static keys, encrypted private
  keys, challenge/response logins.
- **Flatpak and Snap applications** are not tested.

## Reporting a problem

Open an issue on GitHub: https://github.com/rm4n0s/tube/issues. Say what you ran, what you
expected, and what happened; the Tag of the error and the lines of `tube.log` around it help
(see `docs/GUIDE.md`, "The log"). The log has no keys or passwords, but it has the names and the
servers of your profiles: remove what you do not want to show.

For a problem that should not be public before it is fixed (a leak of traffic or of a secret),
write first to the repository's owner, at the address on their GitHub profile
(https://github.com/rm4n0s), and open the issue after the fix.
