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

## Step 5.5 (a): Firefox through the VPN

With a real provider's profile added in the TUI, and a new, empty Firefox profile (Firefox's `-P` makes one: choose "Create Profile…", call it `tube-check`):

```sh
go build -o tube ./cmd/tube
./tube firefox -P tube-check --no-remote
```

Select the profile and press Enter. In that window, open a "what is my IP" page (for example https://ifconfig.me). In a normal Firefox window beside it, open the same page.

Expected: the window of Tube shows the VPN's address, the normal one the machine's own address.

## Step 5.5 (b): a Firefox that already runs

With a normal Firefox already open:

```sh
./tube firefox
```

Select a profile and press Enter.

Expected: no new window opens inside the Firefox that already runs (whose traffic would not go through the VPN): Firefox refuses, or says that it is already running. (Tube's help page says that a program that hands its work to one that already runs is not in the VPN.)

## Step 5.5 (c): adding a real provider's file

```sh
./tube true
```

Press `a`, type the path of a real provider's `.ovpn` file, and press Enter. Then try a file that Tube cannot use, if you have one (for example one with `comp-lzo`, or one that names files that are not next to it).

Expected: the real file is added, and its page (`e`) shows its servers and its login; the errors for a file that Tube cannot use are sentences that say what is wrong and on which line.
