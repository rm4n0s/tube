# Fixtures of the proofs

`three-profiles/` is a configuration folder of Tube with three profiles, `Home`, `Work` (which asks for a user name and a password) and `Lab`, for the proofs of the TUI (`scripts/tui-proof.sh --config scripts/testdata/three-profiles`). It was written by `go run make-three-profiles.go three-profiles` (in this folder, with `TZ=UTC`).

The keys and certificates in these profiles were generated for the proofs, are used nowhere else, and protect nothing. The servers they name do not exist.
