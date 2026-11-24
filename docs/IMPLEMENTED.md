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
