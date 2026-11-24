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

## Step 1.1: The store of profiles
- Date: 2026-11-25
- What was done: `internal/core/profiles`: `Load` (the parser of gopenvpn-conn over an `os.Root`, recording the files a profile names and keeping the limits of 16 files of 1 MiB), `Store` with `Add` (parse, copy, parse the copy, write `meta.json` last), `List`, `Get`, `Rename`, `SetUsername`, `SetDNS`, `MarkUsed`, `Delete`, `CleanName`, `ParseDNS`, and `ProblemOf` for the parser's errors; `profiles/profilestest` (a CA, server and client certificates with ECDSA P-256 keys, and client profiles, made at test time); gopenvpn-conn v1.0.1.
- Checks: `scripts/checks.sh` passed (the tests against the real server are skipped: their script comes with step 1.4)
- Proofs: none
- Manual checks: none
- Differences from the plan or the spec:
  - `Add` does not make the path absolute, so there is no `ProfilePathInvalid`: a relative path works as it is (`os.OpenRoot` takes a relative folder). An empty path is `ProfilePathEmpty`.
  - There is no `ProfileMetaReadFailed`: a `meta.json` that cannot be read is the error of `storage.ReadFile`, returned untouched (trap 4). A `meta.json` of another version is `ProfileMetaNewer`, apart from `ProfileMetaInvalid` (one Tag, one place).
  - More Tags than the plan names, each with its test: `ProfileStatFailed` (the folder of a profile cannot be looked into), `ProfileCopyFolderOpenFailed` (the original's folder is gone before the copy). `ProfileMetaEncodeFailed` and `ProfileRandomFailed` cannot happen and are in `docs/ERROR-TAGS-UNTESTED.md`.
  - An id is 16 lowercase hexadecimal characters; capitals are refused. `List` skips entries of the store's folder that are not folders with such a name. `ErrNotFound` matches `ProfileNotFound` with `errors.Is`.
  - The test of the file limit uses 17 `ca` options (the parser lets `ca` repeat, and each names a file), so the real limit is tested, not a lowered one.
  - `Store` has an unexported hook, `afterLoad`, that the tests use to change the original between the parse and the copy.

## Step 1.2: Editing a profile, and profiles that name scripts
- Date: 2026-11-25
- What was done: `Store.Text` and `Store.ReplaceText`; `Store.Add` takes `AddOptions{WithoutScripts}` and refuses a profile that runs scripts with `ProfileRunsScripts` (metadata `"options"`), or stores it with each script line turned into `# tube: disabled: <the line>`; `withoutScripts`, which reads the profile once and parses its text again after each line it disables (16 at most); `loadWith`, which parses a text given in memory with the files of the folder.
- Checks: `scripts/checks.sh` passed (the tests against the real server are skipped: their script comes with step 1.4)
- Proofs: none
- Manual checks: none
- Differences from the plan or the spec:
  - `ReplaceText` parses the new text in memory with `loadWith`, then writes it with `storage.WriteFileAtomic`. No candidate file, no rename, so no `ProfileTextReplaceFailed`: an atomic write already leaves the old text when it fails.
  - `Add` reads the profile's text once (in `withoutScripts`) and stores that text, so a change of the original after the parse does not reach the store. Reading the profile is now Tube's own error (`ProfileUnreadable`, `ProfileFolderUnreadable`), not the parser's. The test of step 1.1 that changed the original before the copy now damages a named file, which the parse of the copy still catches.
  - `Text` and `ReplaceText` refuse a broken profile (`ProfileBroken`), as SPEC §6.1 says a broken profile can only be deleted.
  - The names of the disabled options are given once each (two `up` lines give `up` once).
