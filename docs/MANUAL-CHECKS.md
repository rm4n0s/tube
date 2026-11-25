# Manual checks

What only a real VPN provider or a real desktop application can show, and the tests therefore cannot (CLAUDE.md "Working through the steps"). Each item names its step and the exact command to run. When you have run one, tell the session what you saw: the answer is written into the step's entry of `docs/IMPLEMENTED.md` and the item is removed from this list.

## Step 3.2: `tube --profile` with a real provider

Build Tube and add a real profile first: run `./tube`, press `a`, type the path of the provider's `.ovpn` file, press Enter, then `q`.

```sh
go build -o tube ./cmd/tube
./tube
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

With your normal Firefox already open (changed on 2026-11-25: Firefox gets a profile of Tube's own, SPEC D28):

```sh
./tube firefox
```

Select a profile and press Enter.

Expected: a second Firefox opens, with none of your bookmarks or logins (it is Tube's own profile), and the Status tab has the line `firefox runs on a profile of its own, which Tube keeps: ...`. A "what is my IP" page (https://ifconfig.me) shows the VPN's address in that window, and the machine's own address in your normal Firefox beside it. `q`, `y` in Tube closes Tube's Firefox only. The second time, Tube's Firefox still has what you did in it the first time.

## Step 5.5 (c): adding a real provider's file

```sh
./tube true
```

Press `a`, type the path of a real provider's `.ovpn` file, and press Enter. Then try a file that Tube cannot use, if you have one (for example one with `comp-lzo`, or one that names files that are not next to it).

Expected: the real file is added, and its page (`e`) shows its servers and its login; the errors for a file that Tube cannot use are sentences that say what is wrong and on which line.

## Step 6.2: the run screen with Firefox

With a real provider's profile:

```sh
go build -o tube ./cmd/tube
./tube firefox -P tube-check --no-remote
```

Select the profile and press Enter. Play a video in that Firefox window. Then press Tab (the Output tab), then Shift-Tab (back to the Status tab: on the Output tab `q` would go to Firefox), and then `q`, `y`.

Expected: the Status tab's rates (`↓` on the first line, and `Received`) move while the video plays; the Output tab shows what Firefox prints; `q`, `y` closes Firefox and Tube ends.

## Step 6.3: switching and disconnecting with Firefox

With two real profiles (of two providers, or two countries of one provider):

```sh
go build -o tube ./cmd/tube
./tube firefox -P tube-check --no-remote
```

Select the first profile and press Enter. In Firefox, open a "what is my IP" page (for example https://ifconfig.me). In Tube, press Tab twice (the Network tab), select the second profile, press Enter, and reload the page. Then press `x`, `y`, and reload the page again.

Expected: the page shows the first VPN's address, then after the switch the second's; after `x` the page does not load; and at no moment does it show the machine's own address.

## Step 7.1 (a): sound and video in a browser

With a real provider's profile:

```sh
./tube firefox -P tube-check --no-remote
```

Select the profile and press Enter. Play a video with sound (a video site, or a web radio) for a few minutes.

Expected: the picture and the sound play as they do without Tube (the sound goes to the machine's speakers: the application shares the sound with the machine, only not its network), and the Status tab's `Received` grows.

## Step 7.1 (b): a long download

With a real provider's profile, a download of a large file (1 GB or more; a Linux distribution's image is a good one):

```sh
./tube --profile <a real profile> curl -L -o /tmp/tube-download.iso <the URL of a large file>
./tube curl -L -o /tmp/tube-download.iso <the URL of a large file>
```

In the second, select the profile and press Enter, and watch the Status tab until the download ends; then remove `/tmp/tube-download.iso`.

Expected: both downloads finish, at about the speed of the VPN without Tube; the Status tab's rate stays above zero while it runs and `Dropped` stays at or near 0; `curl` ends with status 0 (Tube's first line says `curl ended (status 0)`).

## Step 7.1 (c): suspend and resume while an application runs

With a real provider's profile:

```sh
./tube ping 1.1.1.1
```

Select the profile and press Enter, then Tab (the Output tab). Suspend the laptop for a minute or more, and wake it up. Watch the Output tab, then Tab to the Network tab and back to the Status tab.

Expected: after the wake-up, the replies stop for a while; the Status tab's events say `the VPN connection ended (...); the application has no network; connecting again…`, and then `connected again`, and the replies come back without anything typed. While it was not connected, no reply came (the machine's own network was never used). `q`, `y` ends it.

## Step 7.1 (d): a machine where user namespaces are off

On a machine (or a virtual machine) where ordinary users may not make user namespaces, for example a Debian with `sysctl kernel.unprivileged_userns_clone=0`, or an Ubuntu 24.04 with its default `kernel.apparmor_restrict_unprivileged_userns=1`:

```sh
./tube true; echo "status $?"
./tube --profile <any name> true; echo "status $?"
```

Expected: both end at once, with status 125, and with the sentence `This machine does not let an ordinary user make a user namespace, which Tube needs. See docs/GUIDE.md ("What Tube needs"). (SandboxUserNamespaceDenied)`; the TUI does not open, and nothing asks for root.

## The colors and the mouse (a change outside the steps, 2026-11-25): your terminal

The tests click with messages and the proofs through tmux; only your own terminal shows how the colors look and whether its mouse arrives.

```sh
go build -o tube ./cmd/tube
./tube
NO_COLOR=1 ./tube
```

Expected, in the first run: the name `Tube` and the selected row are on violet, the servers blue, the keys of the last line cyan, and everything can be read on your background (try a light and a dark profile of your terminal, if you use both). A click on a profile selects it; the wheel scrolls a long list; a click on `a add` opens the add page, where a double click opens a folder, a click on an `.ovpn` file writes it in the File field, and `[ Cancel ]` goes back; holding `shift` while dragging selects text. In the second run: no color, and the selected row is reversed.

## The colors and the mouse (b): an application through the VPN, with the mouse only

```sh
./tube ping -c 30 example.com
```

Expected: a double click on a real profile connects and runs `ping` (one click only selects it); clicks on `Output`, `Network` and `Status` show the tabs; the wheel scrolls the Output tab and `end` (a click on it) follows again; on the Network tab, a click on `x disconnect` and on `[y] Disconnect` takes the network away, and a double click on the profile gives it back; a click on `q quit` and `[y] Stop and quit` ends it.

## The menu of a field, the stars and the folder of the last add (a change outside the steps, 2026-11-25): your terminal and your clipboard

The tests give the model a clipboard of their own and the proofs a `wl-paste` of their own; only your desktop shows the real one.

```sh
go build -o tube ./cmd/tube
./tube firefox -P tube-check --no-remote
```

Expected: `a` opens the add page on your home folder the first time; after you added a profile from another folder, `a` opens on that folder, also after you started Tube again. On the login page of a profile with a password: copy the user name where your provider shows it (only what is after `Username:`), click the right button in the User name field: a menu with `Paste` opens under the mouse; a click on `Paste` puts the text in the field. The same for the password, which is drawn as stars, as many as it has characters, and never as its characters. With nothing copied (or a picture), the right click opens no menu, and the message line says `The clipboard has no text.` Then `enter`: Tube connects, and the first line shows the rates (`↓`, `↑`) also when the name of the profile is long.

## The remembered password (a change outside the steps, 2026-11-25)

With a real provider's profile that asks for a password:

```sh
./tube firefox
```

Expected: the login page has `[x] remember the password`; after a login that worked, quit, and start again: `enter` on the profile connects at once, without the login page. The profile's page (`e`) says `Password  remembered`; `p`, `y` forgets it, and the next `enter` shows the login page again. `./tube --profile <name> curl -s https://ifconfig.me` prints the VPN's address without asking for anything while the password is remembered.

## A profile file on the command line, and the DNS field (a change outside the steps, 2026-11-25)

The tests do this against a server of their own; only a real provider's file shows it with a real login. Use a provider's `.ovpn` file that is not a profile of Tube yet (or delete its profile in the TUI first).

```sh
go build -o tube ./cmd/tube
./tube --profile <the path of the .ovpn file, with or without .ovpn> curl -s https://ifconfig.me; echo
./tube --profile <the same path> curl -s https://ifconfig.me; echo
./tube
```

Expected, in the first run: `tube: added the profile "<the file's name>"`, then the questions `User name: `, `Password: ` (nothing is shown while you type), `DNS servers (empty: the ones of the VPN): ` (press Enter) and `Save the user name and the password with the profile? [Y/n] ` (press Enter), and then the VPN's address. The second run asks nothing and prints the VPN's address again. In the third, `a` shows the add page with the three fields File, Name and DNS; the profile's page (`e`) of the profile that the first run added says `Password  remembered`.

