# Manual checks

What only a real VPN provider or a real desktop application can show, and the tests therefore cannot (CLAUDE.md "Working through the steps"). Each item names its step and the exact command to run. When you have run one, tell the session what you saw: the answer is written into the step's entry of `docs/IMPLEMENTED.md` and the item is removed from this list.

## Step 3.2: `tube --profile` with a real provider

Build Tube and add a real profile first (until the TUI exists, a profile is added by the tests' code only; this check waits for step 5.3, or for a profile that is already in your store):

```sh
go build -o tube ./cmd/tube
./tube --profile <a real profile> curl -s https://ifconfig.me; echo
curl -s https://ifconfig.me; echo
```

Expected: the first command prints the VPN's address, the second the machine's own address, and the two differ.
