# watui

WhatsApp in the terminal, with vim-style keys and Tokyo Night colors.

It uses [whatsmeow](https://github.com/tulir/whatsmeow) to talk to WhatsApp. That's the same library nchat uses. watui links to your phone as a **linked device**, the same way WhatsApp Web does.

> ⚠️ Unofficial client. Normal personal use has no known bans, but using it goes against WhatsApp's terms, so there is a small risk. Don't use it for bulk messages.

## Screenshots

Rendered from demo data (`make screenshots`). Pictures, stickers and GIFs are drawn by kitty in the real app, so they don't appear here.

| | |
|---|---|
| ![Chat](docs/screenshots/01-chat.png) **Chat** — lazygit-style boxes, replies, reactions, voice notes, files, forwarded messages | ![Emoji](docs/screenshots/02-compose-emoji.png) **Compose** — vim editor with `:emoji` completion |
| ![Message menu](docs/screenshots/03-message-menu.png) **Message menu** — `Space` on a message | ![Chat menu](docs/screenshots/04-chat-menu.png) **Chat menu** — `Space` on a chat |
| ![Group info](docs/screenshots/05-group-info.png) **Group info** — members, admins, description | ![Profiles](docs/screenshots/06-profiles.png) **Profiles** — several WhatsApp accounts |
| ![Keybindings](docs/screenshots/07-help.png) **Keybindings** — `?` | |

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
| Profile box | `Space` | actions: My profile · Turn off/on desktop notifications (per profile, shows ) · Rename · Reconnect · Log out · Remove · Add profile |
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
| | `A` | show archived chats (or `Enter` on the ** Archived (N)** row at the top; **← Back to chats** returns) |
| | `n` | new chat / new group (also `:new`) |
| | `Space` | chat menu: Open · Pin · Mute (8h / 1 week / always) · Archive · Mark read/unread · Copy number · Leave group · Delete chat |
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

### Profile details

Shown in the [3] box in place of the messages (`Esc` goes back), with the profile picture in kitty:

| Open with | Shows |
|---|---|
| Profile box → `Space` → **My profile**, or `:me` | your picture, name, number, about |
| Chats → `Space` on a 1:1 chat → **Contact info** (`i`), or `:info` | picture, address-book name, WhatsApp name, number, about, business, groups in common |
| Chats → `Space` on a group → **Group info** (`i`), or `:info` | picture, name, description, created date/by, settings, members (admins/owner marked) |
| Messages → `Space` on someone's message in a group → **Sender info** | that person's details |

In group info: `j`/`k` move through members, `Enter` opens a chat with them, `Space` shows their details, `y` copies their number. In contact info, `Enter` opens the chat. No picture? A colored avatar with initials is shown instead.

### Emoji and stickers

- **Emoji:** in INSERT, type `:` and a few letters (`:joy`, `:thumbsup`, `:pray`) — a popup suggests emoji (popular ones first). `Tab` inserts, `Ctrl+n`/`Ctrl+p` move, `Esc` hides it. Times like `10:30` don't trigger it.
- **Stickers:** `Ctrl+t` (in Messages or Compose) or `:sticker` opens a grid of stickers you received or sent recently (animated ones move). `h/j/k/l` or `Ctrl+n`/`Ctrl+p` to pick, `Enter` sends it to the open chat.

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
| Open file | `o` | documents: downloads once, opens with the desktop default app |
| View in nvim | `v` | text/code documents (.py .go .sql .json .md .csv …), read-only; `:q` returns |
| Save to ~/Downloads/WhatsApp | `S` | pictures, videos, voice notes, stickers, documents (original file name, never overwrites) |
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
notify = on            # on | private (sender only) | off
notify_sound = watui   # watui (own chime; calls ring) | system | off | /path/to/sound.ogg
```

### Desktop notifications

New messages pop up via `notify-send` (one notification per chat, updated in place). Clicking it opens that chat in watui (switching profile if needed) and brings the tmux pane and kitty window to the front. No notification for muted or archived chats, for the chat you are reading while watui is focused, or for old messages arriving after a reconnect. `notify = private` shows only who wrote; `:set notify=off` mutes them for the session.

Calls: watui can't answer calls (whatsmeow has no audio/video), but:

- a ringing call shows a red banner at the bottom (`Ctrl+x` declines it) and a notification with a **Decline** button;
- every call is logged in the chat: green *Voice call · answered on another device*, orange *Declined*, red *Missed* (missed ones count as unread).

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

Sending new images, files or voice notes, playing video inside the terminal, Lottie (vector) stickers, voice/video calls, showing which message is pinned.

Video and voice notes need `mpv`; `ffmpeg` grabs video frames and animates GIFs inline; ImageMagick (`convert`) animates stickers: `sudo apt install mpv ffmpeg imagemagick`.

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
