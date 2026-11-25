# Tube: the guide

Tube runs one application through a VPN, and only that application. This guide is for the people
who use it. `README.md` says what Tube is, and how to build and install it.

## What Tube needs

Tube makes a private user, network and mount namespace for the application. It does that as an
ordinary user, so the kernel must allow it. Three settings decide it:

| Setting | Needed value | Where it exists |
|---|---|---|
| `user.max_user_namespaces` | above 0 | every kernel |
| `kernel.unprivileged_userns_clone` | `1` | Debian and its derivatives |
| `kernel.apparmor_restrict_unprivileged_userns` | `0`, or an AppArmor profile that allows Tube | Ubuntu 24.04 and later |

To see them:

```
sysctl user.max_user_namespaces
sysctl kernel.unprivileged_userns_clone
sysctl kernel.apparmor_restrict_unprivileged_userns
```

A setting that your kernel does not have makes `sysctl` say "cannot stat"; that setting then
does not matter. Changing a setting needs root (`sudo sysctl -w ...`). Tube itself never asks
for root, and never changes these settings.

Tube also needs `/dev/net/tun` that you may open. On the usual distributions it has mode 0666.

Tube checks both when it starts, before it shows anything. When the kernel refuses, Tube ends
with status 125 and says:

```
tube: This machine does not let an ordinary user make a user namespace, which Tube needs. See docs/GUIDE.md ("What Tube needs"). (SandboxUserNamespaceDenied)
```

## Adding a profile

Start Tube, with or without an application:

```
tube firefox
tube
```

The first page is the list of your profiles. Press `a` to add one (or click on `a add` on the
last line: see "The mouse").

The add page shows **a folder**: `../` (the folder above), its folders, and its `.ovpn` files.
The first time it is your home folder; afterwards it is the folder of the profile that you added
last, also after you started Tube again. Walk to the file:

- With the mouse: a double click opens a folder; a click on a folder of the path on the upper
  edge of the box goes back to it; the wheel scrolls. A click on the `.ovpn` file writes it in
  the **File** field. Then click on `[ Add ]`.
- With the keys: `↑` and `↓` move, `enter` opens a folder, `backspace` goes to the folder above.
  `enter` on the `.ovpn` file writes it in the File field and goes to the **Name** field; `enter`
  there adds the profile.

You can also type or paste the path: text that you type goes to the File field (`~` is your home
folder, and `tab` completes the name of a file or a folder there), and the list follows what you
type. In the Name field, type a name if you want another one than the file's name, which the
field shows. In the **DNS** field you can give the profile DNS servers of its own, up to 4 IP
addresses separated by spaces or commas (see "DNS servers of a profile"); leave it empty to use
the ones that the VPN sends, as most profiles do. `↑`, `↓` and `tab` go from one part of the
page to the next.

There is a way without the terminal UI too: `tube --profile ~/Downloads/work.ovpn firefox` adds
the file and runs the application at once (see "Using Tube in scripts, and for programs that
need the whole terminal").

The list shows only folders and files whose name ends in `.ovpn`, and no hidden ones. To see
every file (a profile that is called `client.conf`, a hidden folder), turn on `[ ] all files`
with a click, or with `space` when it has the focus.

Tube keeps a copy of the profile. It copies the `.ovpn` file and the files that it names, so the
profile keeps working when you move or delete the file that you added it from. The files that a
profile names (`ca`, `cert`, `key`, `tls-auth`, `tls-crypt`, ...) must be next to it, or below
its folder. A profile can also hold them inside itself (`<ca>...</ca>`).

Some profiles **run scripts**: they have options such as `up` or `down`. These name a program
that the official client runs when it connects, usually to change the DNS of the machine. Tube
never runs a program that a profile names, and it sets the DNS of the application itself. When
a profile runs scripts, Tube asks:

```
This profile runs scripts (up, down). Tube never runs them, and sets the DNS of the application itself. Add it without them?
```

Answer `y` to add it with those lines turned into comments (`# tube: disabled: ...`). The file
that you added it from is not changed.

The keys of the list:

| Key | What it does |
|---|---|
| `↑` `↓` `k` `j`, `home` `end`, `pgup` `pgdn` | move |
| `enter` | with an application: connect and run it. Without one: open the profile's page |
| `a` | add a profile |
| `e` | open the profile's page |
| `d` | delete the profile, after a question |
| `?` | the keys of every page |
| `q`, `ctrl+c` | quit |
| a click, a double click | select the profile; a double click is `enter` |

The profile's page (`e`) shows everything about the profile: its servers, whether it asks for a
password, the remembered user name, the DNS servers, the files, and the options that Tube
ignores. Its keys:

| Key | What it does |
|---|---|
| `r` | rename |
| `u` | the user name to remember |
| `p` | forget the remembered password (only when one is remembered) |
| `n` | the DNS servers (see "DNS servers of a profile") |
| `t` | edit the text of the profile in your editor (`$VISUAL`, else `$EDITOR`, else `nano` or `vi`) |
| `d` | delete |
| `enter` | with an application: connect and run it |
| `esc` | back to the list |

After `t`, Tube checks the text that you saved the way it checks a new profile. When the text is
not valid, the profile stays as it was, and the message says why.

## The mouse

Everything in Tube can be done with the keys, and with the mouse:

- A **click** selects a row of a list, shows a tab, presses a button such as `[ Add ]` or
  `[y] Delete`, puts the cursor in a field, and changes a mark such as `[x] remember the user
  name`. A click on a key of the last line (`a add`, `? keys`, `esc back`) presses that key.
- A **double click** on a row is `enter` on it: it runs (or opens) a profile, uses a profile on
  the Network tab, and opens a folder on the add page. One click never connects or runs
  anything.
- The **wheel** scrolls a list, and the Output tab.
- A click of the **right button** in a text field opens a menu with `Paste`, when the clipboard
  has text (see "A password").

While Tube is open, the mouse belongs to it. To select text with the mouse, to copy it, hold
`shift` while you drag: your terminal then selects, as it does in other full-screen programs.

Tube has colors for a dark and for a light terminal, and asks your terminal which one it is. It
follows `NO_COLOR`: without colors, the selected row is drawn reversed.

## Running an application

```
tube firefox
```

Select a profile and press `enter`. A profile that asks for a password shows the login page
first (see "A password"). Tube connects, starts the application, and shows the run screen.

The first line says what runs, through which profile, the state of the VPN, and the traffic of
the last second, down (`↓`) and up (`↑`). The second line has the tabs. `tab` goes to the next
tab and `shift+tab` to the one before; a click on a tab shows it.

- **Status**: the application and whether it runs; the VPN and its state; the address, the DNS
  servers and the MTU of the tunnel; the bytes and packets each way, and the rates; the dropped
  packets; and Tube's events, such as "connected again", with their time.
- **Output**: what the application prints, as plain lines. It follows the end; `pgup`, `pgdn`
  and the wheel scroll, `home` goes to the oldest line that is kept, and `end` follows again. **Every other key
  goes to the application**, as if you typed it on its terminal: text, `enter`, `backspace`,
  `ctrl+c`, `ctrl+d`, `ctrl+u`, `esc`, and pasted text. So `ctrl+c` on this tab stops `ping`,
  and you can answer a program that asks a question.
- **Network**: your profiles, with `*` and `in use` on the one that the application uses, and a
  line that says what the application has now. Press `enter` on another profile, or double-click
  it, to **switch** to it. Tube connects the new profile first; the application keeps the old one until the new one is
  connected. When the new one fails, nothing changes. `esc` cancels a switch that is still
  connecting. Press `x` to **disconnect**: the application keeps running, without network, until
  you choose a profile with `enter`.

To **quit**, press `q` (or `ctrl+c`) on the Status or the Network tab. While the application runs,
Tube asks:

```
firefox is still running. Stop it and quit?
```

On `y`, Tube sends the application SIGTERM, and SIGKILL five seconds later when it still runs,
and quits when it has ended. A second `ctrl+c` quits at once.

**When the application ends, Tube ends too.** Close the window of Firefox, or let `ping -c 3`
finish, and the terminal UI closes by itself; you do not have to press a key. Your terminal then
shows the last 20 lines that the application printed, and how it ended:

```
3 packets transmitted, 3 received, 0% packet loss, time 2003ms
rtt min/avg/max/mdev = 11.9/12.4/12.9/0.4 ms
tube: ping ended (status 0)
```

Tube exits with the application's exit status.

## Firefox and other programs that are already open

Some programs do not do the work themselves when one of them already runs. They ask the one
that runs to do it. A second `firefox` asks the first one to open a window. A terminal emulator
asks its server. The process that does the work is outside Tube's namespace, so **its traffic is
not in the VPN**.

**For Firefox (and Thunderbird), Tube takes care of it.** `tube firefox` always starts a Firefox
of its own, on a Firefox profile that Tube keeps for it, so it is a separate process inside the
VPN, also while your normal Firefox is open. Tube says so in a line of the Status tab:

```
firefox runs on a profile of its own, which Tube keeps: it is apart from a firefox that is already open
```

What this means for you:

- Tube's Firefox has its own history, logins, bookmarks and settings. The ones of your normal
  Firefox are not in it, and what you do in it stays apart from your normal Firefox. It is kept
  between runs, in Tube's state folder (`firefox-profile`, see "Where Tube keeps its files").
- Your normal Firefox, open beside it, keeps using the machine's own network.
- To use a Firefox profile of your choice, name it: `tube firefox -P work`. Tube then adds
  nothing. But when a Firefox is already open **on that same profile**, Firefox opens the window
  there, outside the VPN, and Tube cannot stop it: close that Firefox first.

Tube cannot know this for every other program. When you are not sure, quit the program
everywhere before you start it with Tube.

## DNS servers of a profile

The application looks names up at the DNS servers of the VPN, through the tunnel. The machine's
own resolver is not visible to it. Some VPN servers send no DNS server. Tube then warns:

```
the VPN gave no DNS server: names will not be found; set DNS servers for the profile
```

Give the profile DNS servers of its own: up to 4 IP addresses, separated by spaces or commas
(for example `10.0.0.53, 1.1.1.1`). These are then used in place of the VPN's, and they are
reached through the tunnel like everything else. An empty field means "the ones that the VPN
sends". There are three places for it:

- the **DNS** field of the add page, when you add the profile;
- the profile's page afterwards: press `n`;
- `--dns` on the command line, with `--profile`: for that run only, or with `--save` for the
  profile (see "Using Tube in scripts, and for programs that need the whole terminal").

## A password

A profile that asks for a user name and a password opens the login page before it connects. The
user name is filled in when it was remembered, and "remember the user name" is on by default
(`space` turns it). The password is drawn as stars, one `*` for each character, never as the
characters.

**The password is remembered too**, when "remember the password" is on (it is, by default) and
the login worked. The next time, Tube connects that profile at once, without the login page,
also with `tube --profile <name>`. Tube keeps the password in a file that only you can read
(mode 600), in the folder of the profile, the way it keeps the profile's private key: it is not
encrypted, and who can read your files can read it. To have Tube ask each time, turn the mark
off before you connect. To forget a remembered password, open the profile's page (`e`) and
press `p`: its line says `Password  remembered`, or `asked each time`. When the server refuses a
remembered password (you changed it at your provider), the login page opens again for the new
one.

You do not have to type them. Copy the user name (or the password) where your provider shows
it, **click the right button of the mouse in the field**, and click `Paste` in the menu that
opens (or press `enter`). Do the same for the other field. The menu opens only when the
clipboard has text, and it never shows that text. Tube reads the clipboard with `wl-paste` (the
package `wl-clipboard`) on Wayland, or with `xclip` or `xsel` on X11; without one of them it
asks your terminal, and many terminals do not answer: the message line then says so, and you
paste with your terminal's own key (often `ctrl+shift+v`), which always works. Every other text
field has the same menu.

A provider's page often shows the two as `Username: ...` and `Password: ...`. Give only what is
after the label: with the label, the server refuses the login. The same holds for the file of
`--auth-file`: the user name alone on the first line, the password alone on the second.

With `--profile`, Tube asks on the terminal: the user name when none is remembered, then the
password, without echo, and then whether to save them (see the next section). For scripts, give
a file with the user name on the first line and the password on the second:

```
tube --profile work --auth-file ~/.config/work-vpn.auth curl https://example.com
```

The file must be readable by you only (`chmod 600`); Tube refuses it otherwise. `--username`
and `--password` give them as options; read what the next section says about `--password`
first.

## Using Tube in scripts, and for programs that need the whole terminal

`--profile <name>` (`-p`) connects that profile and runs the application without the terminal
UI. The application gets your terminal, so full-screen programs work:

```
tube -p work ssh host
tube -p work htop
```

The name is the profile's exact name, or the only one that has it when the case is ignored.
While the application runs, Tube writes its own lines on standard error, each starting with
`tube: `. `--quiet` (`-q`) leaves them out.

**A profile file in place of a name.** `--profile` also takes the path of an `.ovpn` file, and
you may leave the `.ovpn` out:

```
tube -p ~/Downloads/work.ovpn curl https://example.com
tube -p ~/Downloads/work curl https://example.com
```

The first time, Tube adds the file as the profile `work` (it says `tube: added the profile
"work"`) and runs the application. Every time after, the same command uses that profile; so
does `tube -p work ...`, from any folder. Tube looks for a profile with that name first, and
for a file only when there is none. A file that runs scripts is added without them after you
answered `y` on the terminal.

**What is missing is asked for.** When the profile needs a user name and a password and has
none remembered, and when a profile that was just added has no DNS servers, Tube asks on the
terminal, one question after the other, and then whether to keep the answers:

```
$ tube -p ~/Downloads/work.ovpn curl https://example.com
tube: added the profile "work"
User name: ada
Password:
DNS servers (empty: the ones of the VPN): 10.0.0.53
Save the user name, the password and the DNS servers with the profile? [Y/n]
```

`enter` alone answers the DNS question with "the ones of the VPN", and the last question with
yes. After a yes the next run asks nothing. After a no the profile stays as it was: the answers
were for this run. The DNS servers are asked for only together with another question or for a
profile that was just added, so a profile that needs no answer runs without one.

**Without questions.** Give the same as options, for a script or for one command line:

| Option | What it gives |
|---|---|
| `--username <name>` | the user name |
| `--password <password>` | the password |
| `--auth-file <path>` | both, from a file that only you can read (see "A password") |
| `--dns <addresses>` | the DNS servers of the application for this run, in place of the profile's: `--dns 10.0.0.53,1.1.1.1`. `--dns ""` is the ones that the VPN sends |
| `--save` | keep the user name, the password and the DNS servers of this run with the profile, without the question |

```
tube -p ~/Downloads/work.ovpn --username ada --password 'the password' --dns 10.0.0.53 --save true
tube -p work curl https://example.com
```

The first line adds the profile and saves everything (`true` is a program that does nothing);
the second, and every run after it, needs no option and asks nothing. Without `--save`, options
change nothing of the profile. A user name and a password are saved only when the server
accepted them. Where there is no terminal (a `cron` job, a service), Tube cannot ask: it ends
with an error that names these options when a user name or a password is missing, and it uses
the DNS servers of the VPN without asking.

**`--password` can be seen by others.** An option is part of the command line, and the other
users of your machine can read the command lines of running programs (`ps`); your shell may
also write it to its history. On a machine that you share, use `--auth-file`, or let Tube ask
once and save the answer.

The exit status is the application's. Besides that:

| Status | Meaning |
|---|---|
| 2 | a mistake in the arguments |
| 125 | Tube itself failed; the message says why |
| 126 | the application cannot be executed |
| 127 | the application was not found |
| 130 | the terminal UI was quit before a profile was selected |
| 128 + n | the application was ended by signal n (143 for SIGTERM) |

`tube --version` prints the version, and `tube --help` the options.

## When the VPN drops

When the VPN connection ends by itself, the application keeps running **with no network**. Tube
never lets it use the machine's network in between. Tube connects the same profile again, after
1, 2, 4, 8 and 15 seconds, and then every 30 seconds. Its events (on the Status tab, or on
standard error with `--profile`) say:

```
tube: the VPN connection ended (the server did not answer); the application has no network; connecting again…
tube: connected again
```

When the server refuses the user name or the password on such a try, Tube stops trying:

```
tube: the VPN server refused the user name or the password; the application stays without network
```

Then choose the profile again on the Network tab. After you disconnected with `x`, Tube does not
connect again by itself.

## What the errors mean

An error is shown as a sentence, followed by its Tag in parentheses, for example
`The VPN server did not answer. (ServerPollTimeout)`. The Tag helps when you report a problem;
the log (see "The log") has the whole error. The sentences, and what to do:

**The VPN**

- "The VPN server refused the user name or the password." Check them, and type the password
  again.
- "The VPN server did not answer." The server is down, or your network blocks it. Try another
  server of the provider, or the profile's TCP variant.
- "The certificate of the VPN server was not accepted." The profile does not match the server.
  Download the profile again from the provider.
- "The VPN server closed the connection." The server ended the session. Connect again.
- "The VPN server asks for something that Tube does not support." The server needs an option
  that gopenvpn-conn does not have. The log names it.
- "Cancelled." You cancelled the connect.

**Profiles**

- "This profile needs a user name and a password." Give them on the login page, or with
  `--auth-file`, or with `--username` and `--password`.
- "This profile runs scripts (<options>). Tube never runs them." and "This profile runs scripts.
  Tube never runs them." Add it in the terminal UI, or with `--profile <file>` on a terminal,
  and answer `y` to add it without them.
- "Another profile is already called "<name>"." Choose another name; names are compared without
  case.
- "A profile needs a name." Type a name.
- "The name is too long (64 characters at most)." Choose a shorter name.
- "The file "<file>" of the profile is too large." A file that the profile names is larger than
  1 MiB. It is probably not the file that the profile means.
- ""<value>" is not an IP address." The DNS servers are IP addresses, such as `10.0.0.53`.
- "A profile can have 4 DNS servers at most." Remove some.
- "There is no profile called "<name>"." Check the name of `--profile`; `tube` without
  arguments shows the list.
- "There is no profile called "<name>", and no .ovpn file at that path." What `--profile` got is
  neither the name of a profile nor the path of a file, with or without `.ovpn`. Check the
  spelling, and the folder that you are in when the path is not a whole one.
- "More than one profile is called "<name>" when the case is ignored. Write the name exactly."
  Write the name with the same capitals as in the list.

**The parser of profiles**

- "The profile uses the option "<option>" on line <line>, which Tube does not support:
  <reason>." Tube does not support this option (compression, a proxy, a static key, ...). Ask
  the provider for a profile without it, or remove the line when the server does not need it.
- "The option "<option>" on line <line> of the profile is not valid: <reason>." The line has a
  mistake. Fix it with `t` on the profile's page, or in the file before you add it.
- "The profile has no "<option>"." The profile lacks something that a connection needs, usually
  `remote`. Download it again.
- "A file that the profile names with "<option>" cannot be used. Put it next to the profile, or
  inside the profile." The file is missing, or outside the profile's folder.
- "The profile cannot be read: a mistake on line <line>." The file is not a valid profile at
  that line.
- "The profile is too large." A profile is at most 1 MiB.

**The machine and the application**

- "This machine does not let an ordinary user make a user namespace, which Tube needs." See
  "What Tube needs".
- "Tube cannot open /dev/net/tun." The device is missing or you may not open it. On most
  machines `ls -l /dev/net/tun` shows `crw-rw-rw-`; ask the machine's administrator otherwise.
- "The program "<name>" was not found." and "The program was not found." The application is not
  in your `PATH`. Check its name, or give its full path.
- "The program cannot be executed." The file is not executable (`chmod +x`), or not a program.

Any other error shows its own message. When it is not clear, look at the log, and report it
(`docs/SECURITY.md` says where).

## Where Tube keeps its files

| Folder | Where | What is in it |
|---|---|---|
| Configuration | `$TUBE_CONFIG_DIR`, else `$XDG_CONFIG_HOME/tube`, else `~/.config/tube` | your profiles: `profiles/<id>/meta.json`, `profiles/<id>/files/...`, and `profiles/<id>/password` when the password is remembered |
| State | `$TUBE_STATE_DIR`, else `$XDG_STATE_HOME/tube`, else `~/.local/state/tube` | the log; `state.json` (the folder of the last add); `firefox-profile/` and `thunderbird-profile/` (Tube's own profiles of these programs) |
| Temporary | a new folder under `$XDG_RUNTIME_DIR`, else under the system's temporary folder | what the namespace needs while an application runs; removed at the end |

Every folder that Tube makes has mode 0700 and every file mode 0600. Several Tubes may run at
once, each with its own VPN connection.

## The log

Tube writes a log to `tube.log` in the state folder, as JSON lines. When it passes 1 MiB it
becomes `tube.log.1`. The log has the start and the end of each run, the profile, the addresses
and DNS servers of the tunnel, the traffic counters, and every error with its Tag and where it
happened. It never has a key, a password, a token, or a line of a profile. Nothing is logged to
the terminal.
