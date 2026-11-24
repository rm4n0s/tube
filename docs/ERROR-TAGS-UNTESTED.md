# Error Tags without a test

Every error Tag has a test that asserts its route (CLAUDE.md "Definition of done"). The Tags listed here are the only exceptions: failures that a test cannot make happen. `scripts/error-tags.sh --untested` leaves them out, and a test of `internal/archtest` fails when a Tag is listed here although a test asserts it. One line per Tag, as "- `Tag`: reason".

- `NetlinkBindFailed`: binding a netlink socket with port id 0 lets the kernel choose a free one, which does not fail
- `NetlinkSocketFailed`: `socket(AF_NETLINK, SOCK_RAW, NETLINK_ROUTE)` fails only when the process is out of file descriptors or memory, which a test cannot cause without breaking the rest of the test process
- `ProfileMetaEncodeFailed`: `json.MarshalIndent` of `meta`, a struct of strings, an int and times, cannot fail
- `ProfileRandomFailed`: `crypto/rand.Read` never returns an error since Go 1.24; it crashes the program instead
