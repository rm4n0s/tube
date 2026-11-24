# Error Tags without a test

Every error Tag has a test that asserts its route (CLAUDE.md "Definition of done"). The Tags listed here are the only exceptions: failures that a test cannot make happen. `scripts/error-tags.sh --untested` leaves them out, and a test of `internal/archtest` fails when a Tag is listed here although a test asserts it. One line per Tag, as "- `Tag`: reason".

- `ProfileMetaEncodeFailed`: `json.MarshalIndent` of `meta`, a struct of strings, an int and times, cannot fail
- `ProfileRandomFailed`: `crypto/rand.Read` never returns an error since Go 1.24; it crashes the program instead
