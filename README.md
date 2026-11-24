# Tube

Tube runs one application through a VPN, and only that application:

```
tube firefox
tube ping -c 3 example.com
```

It opens a terminal UI with your OpenVPN profiles (`.ovpn`): add, look at, change and delete
them there, and select one. Tube connects to it and starts the application so that all of its
network goes through the VPN, and nothing else of the machine does. It needs no root and no
installed `openvpn`: the application runs in a private network namespace whose only way out is
the VPN.

**Work in progress.** `docs/SPEC.md` is the specification, `docs/IMPLEMENTATION.md` the plan,
and `docs/IMPLEMENTED.md` the log of what is done. Linux only.

Tube connects to OpenVPN servers with [gopenvpn-conn](https://github.com/rm4n0s/gopenvpn-conn).
It is not OpenVPN's own program and is not affiliated with OpenVPN Inc; OpenVPN is a trademark
of OpenVPN Inc.

## License

GPL-2.0-only. See `LICENSE`.
