# Error Tags without a test

Every error Tag has a test that asserts its route (CLAUDE.md "Definition of done"). The Tags listed here are the only exceptions: failures that a test cannot make happen. `scripts/error-tags.sh --untested` leaves them out, and a test of `internal/archtest` fails when a Tag is listed here although a test asserts it. One line per Tag, as "- `Tag`: reason".

- `KeeperMountPrivateFailed`: the keeper owns its new mount namespace, and making its mounts private there does not fail
- `NetlinkBindFailed`: binding a netlink socket with port id 0 lets the kernel choose a free one, which does not fail
- `NetlinkSocketFailed`: `socket(AF_NETLINK, SOCK_RAW, NETLINK_ROUTE)` fails only when the process is out of file descriptors or memory, which a test cannot cause without breaking the rest of the test process
- `ProfileMetaEncodeFailed`: `json.MarshalIndent` of `meta`, a struct of strings, an int and times, cannot fail
- `ProfileRandomFailed`: `crypto/rand.Read` never returns an error since Go 1.24; it crashes the program instead
- `SandboxMessageEncodeFailed`: `json.Marshal` of a message, a struct of strings and a map of strings, cannot fail
- `SandboxReceiveFailed`: `recvmsg` on a socket pair fails only when the socket is closed, which has its own Tag (`SandboxSocketClosed`)
- `SandboxSocketFailed`: `net.FileConn` of a socket of AF_UNIX fails only when the process is out of file descriptors
- `SandboxSocketPairFailed`: `socketpair(AF_UNIX, SOCK_SEQPACKET)` fails only when the process is out of file descriptors or memory
