# watui

WhatsApp in the terminal, with vim-style keys and Tokyo Night colors.

It uses [whatsmeow](https://github.com/tulir/whatsmeow) to talk to WhatsApp. That's the same library nchat uses. watui links to your phone as a **linked device**, the same way WhatsApp Web does.

> ⚠️ Unofficial client. Normal personal use has no known bans, but using it goes against WhatsApp's terms, so there is a small risk. Don't use it for bulk messages.

## Install

You need Go 1.23+ (`/usr/local/go`) and gcc (for SQLite).

```bash
make deploy         # tests, builds, copies to ~/App/WaTui and links ~/.local/bin/watui
```

## First run

```bash
watui
```

It shows a QR code. On your phone: **WhatsApp → Settings → Linked devices → Link a device**, then scan it. Your chats load in the background ("syncing" in the status bar).

Other commands:

```bash
watui login     # link the default profile again (after logout)
watui add work  # link another account as profile "work"
watui logout    # unlink this computer
```

## Profiles (several WhatsApp accounts)

All profiles stay connected; the screen shows one at a time. The top-left **[1] Profile** box shows the current profile and the unread count of the others.

| Where | Key / command | Action |
|---|---|---|
| anywhere | `gt` / `gT` | next / previous profile |
| anywhere | `1` (or `Ctrl+k` from Chats) | focus the Profile box |
| Profile box | `Enter` | list of profiles (unread per profile) → pick with `Ctrl+n`/`Ctrl+p` + `Enter`, or its number |
| Profile box | `Space` | actions: Rename · Reconnect · Log out · Remove · Add profile |
| Profile box | `a` | add a profile (QR code) |
| command | `:profile <name>`, `:profile add <name>`, `:profile rename <name> <label>` | same from the command line |

From the shell: `watui add work`, `watui profiles`, `watui logout work`. Your first account is the `default` profile (data stays in `~/.local/share/watui`); others live in `~/.local/share/watui/profiles/<name>/`. Remove deletes a profile's saved chats; `default` can only be logged out.

## Keys

The screen has two panes: **chat list** (left) and **messages** (right). `Tab` switches between them. Press `?` inside the app for the full list.

| Mode | Key | Action |
|---|---|---|
| NORMAL (list) | `j` / `k` | move down / up |
| | `gg` / `G` | first / last chat |
| | `Ctrl+d` / `Ctrl+u` | half page |
| | `Enter` | open chat |
| | `i` | open chat and type |
| | `/` | search chats |
| | `A` | show archived chats |
| | `n` | new chat / new group (also `:new`) |
| NORMAL (messages) | `j` / `k` | select next / previous message |
| | `Ctrl+d` / `Ctrl+u`, `gg` / `G` | jump |
| | `Space` | message menu (like right-click) |
| | `i` / `a` / `Enter` | type a message |
| | `e` | write the message in `$EDITOR` (nvim) |
| | `q` / `Esc` | back to the list |
| anywhere | `Ctrl+h` / `Ctrl+l` | Chats / Messages box; at the edge, the tmux pane next door |
| | `Ctrl+j` / `Ctrl+k` | Messages ↔ Compose (Compose opens in NORMAL; from Compose only in NORMAL); at the edge, the tmux pane above/below |
| | `J` / `K` | next / previous chat |
| | `:` | command line |
| Compose | `Ctrl+s` | send (from any vim mode) |
| | `Ctrl+e` | edit in `$EDITOR` |

### Compose is a small vim

| Mode | Keys |
|---|---|
| INSERT | type; `Enter` new line; `Esc` or `jk` → NORMAL; `Ctrl+w` delete word; `Ctrl+u` delete to line start |
| NORMAL | `h j k l w b e 0 ^ $ gg G` move · `i a I A o O` insert · `x X dd D dw de diw daw` delete · `cw ce ciw cc C s S` change · `yy Y yiw p P` yank/paste · `u` / `Ctrl+r` undo/redo · `J` join · `~` case · `r` replace · `v` / `V` visual · `Esc` leaves Compose |
| VISUAL / V-LINE | move to extend · `d` / `x` delete · `y` yank · `c` change · `~ u U` case · `o` other end · `p` replace with yank · `Esc` back |

### New chat and new group

`n` in Chats opens a search over your synced contacts. Type part of a name or a number (a leading `0` is read as your own country code), `Ctrl+n`/`Ctrl+p`, `Enter`.

- Picking a contact opens the chat and starts typing (an existing chat with them just opens).
- Typing a number that isn't a contact shows **Chat with +62…** first; watui checks it is on WhatsApp before opening it.
- **+ New group…**: `Enter` toggles members (search keeps working), then **✓ Next** asks for the name (max 25 characters) and creates the group.

### Message menu

Select a message with `j`/`k`, press `Space`. Move with `Ctrl+n`/`Ctrl+p`, pick with `Enter` (or press the letter), close with `Esc`.

| Option | Key | Notes |
|---|---|---|
| Expand / Collapse | `z` | only on long messages (folded after `collapse_lines`) |
| Reply | `r` | the quote shows in the Compose title; `Esc` (in normal mode) cancels |
| Edit | `e` | your own text messages, first 15 minutes |
| React | `+` | 👍 ❤️ 😂 😮 😢 🙏 🔥 🎉, or remove yours |
| Play video | `o` | downloads once, opens in an mpv window (GIFs loop); Celluloid/xdg-open if no mpv |
| Play / Pause voice note | `o` | plays in the background with mpv; progress bar on the message and in the status box |
| Speed | `.` | 1× → 1.5× → 2× while a voice note plays |
| Stop | `x` | stop the voice note |
| Save to ~/Downloads/WhatsApp | `S` | pictures, videos, voice notes, stickers |
| Show / Hide image | `v` | shows pictures that are hidden (older than the newest 40, or `images=off`/`thumbnail`) |
| Open image in viewer | `o` | opens the downloaded picture with `xdg-open` |
| Copy text | `y` | via `xclip` |
| Forward | `f` | type to search a chat, `Enter` to send |
| Star / Unstar | `s` | ★ shows next to the message |
| Pin | `p` | 24 hours, 7 days or 30 days, or unpin |
| Delete for everyone | `d` | your own messages, first 2 days; asks first |

Commands: `:pause` `:speed` `:stop` (playing voice note), `:pin` `:unpin` `:mute` `:unmute` `:archive` `:unarchive` `:archived` `:read` `:q`

## Settings

`~/.config/watui/config` is created on first run:

```
images = always        # always | thumbnail | off
image_width = 40       # biggest picture size, in terminal cells
image_height = 15
collapse_lines = 15    # long messages fold after this many lines (0 = never)
```

| `images` | What you see |
|---|---|
| `always` | full picture inline (downloads it; newest 40 per chat) |
| `thumbnail` | the small blurry preview WhatsApp sends with the message, no download |
| `off` | text only, e.g. `[image] caption` |

Change a setting while watui is running with `:set images=thumbnail` (that only lasts until you quit), or type `:set` to see the current values.

Pictures need kitty. Inside tmux they also need `set -g allow-passthrough on` in your tmux config.

With vim-tmux-navigator, tmux steals `Ctrl+h`/`Ctrl+l` unless watui is in its vim pattern. Add `watui` to the pattern by rebinding `C-h`/`C-j`/`C-k`/`C-l` after the `run tpm` line (see `~/.tmux.conf`).

## Where things are stored

`~/.local/share/watui/`:

| File | What |
|---|---|
| `whatsapp.db` | your link keys (anyone with this file can read your chats, so keep it private) |
| `messages.db` | message history shown in the app |
| `watui.log` | errors (`WATUI_DEBUG=1 watui` logs more) |

Downloaded pictures are cached in `~/.cache/watui/images/` (safe to delete).

## Not supported yet

Sending new images, files or voice notes, playing video inside the terminal, animated stickers, voice/video calls, desktop notifications, showing which message is pinned.

Video and voice notes need `mpv` (and `ffmpeg` for grabbing a sharp video frame): `sudo apt install mpv ffmpeg`.

## Keep it updated

WhatsApp blocks old client versions, so update the library every few months:

```bash
go get go.mau.fi/whatsmeow@latest && go mod tidy && make install
```

## Project layout

```
cmd/watui/         entry point: login (QR), logout, starts the TUI
internal/wa/       whatsmeow wrapper: events → local store, send/edit/delete/react/forward/star/pin
internal/store/    SQLite message store (messages.db)
internal/media/    picture download, cache and kitty graphics (Unicode placeholders, tmux passthrough)
internal/config/   ~/.config/watui/config
internal/ui/       Bubble Tea UI: panels, chat list, messages, message menu, vim compose editor
```

## Development

```bash
make test       # go test ./...
make build      # bin/watui
make deploy     # ~/App/WaTui/watui + ~/.local/bin/watui symlink
```

Two commands:

| Command | Runs |
|---|---|
| `watui` | the deployed build in `~/App/WaTui` (update it with `make deploy`) |
| `watuidev` | the code in this folder, rebuilt on every start (script in `~/.local/bin/watuidev`) |

Both use the same login and chats (`~/.local/share/watui`), so only one can run at a time. If one is already open, starting another shows where it is running (process, terminal, tmux window) and asks whether to close it and open here: it is asked to quit cleanly first, and forced after 5 seconds.
