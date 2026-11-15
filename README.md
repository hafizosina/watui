# watui

WhatsApp in the terminal, with vim-style keys and Tokyo Night colors.

It uses [whatsmeow](https://github.com/tulir/whatsmeow) to talk to WhatsApp. That's the same library nchat uses. watui links to your phone as a **linked device**, the same way WhatsApp Web does.

> ⚠️ Unofficial client. Normal personal use has no known bans, but using it goes against WhatsApp's terms, so there is a small risk. Don't use it for bulk messages.

## Install

You need Go 1.23+ (`/usr/local/go`) and gcc (for SQLite).

```bash
make install        # builds and copies the binary to ~/.local/bin/watui
```

## First run

```bash
watui
```

It shows a QR code. On your phone: **WhatsApp → Settings → Linked devices → Link a device**, then scan it. Your chats load in the background ("syncing" in the status bar).

Other commands:

```bash
watui login     # link again (after logout)
watui logout    # unlink this computer
```

## Keys

The screen has two panes: **chat list** (left) and **messages** (right). `Tab` switches between them. Press `?` inside the app for the full list.

| Mode | Key | Action |
|---|---|---|
| NORMAL (list) | `j` / `k` | move down / up |
| | `gg` / `G` | first / last chat |
| | `Ctrl+d` / `Ctrl+u` | half page |
| | `Enter` / `l` | open chat |
| | `i` | open chat and type |
| | `/` | search chats |
| | `A` | show archived chats |
| NORMAL (messages) | `j` / `k` | select next / previous message |
| | `Ctrl+d` / `Ctrl+u`, `gg` / `G` | jump |
| | `Space` | message menu (like right-click) |
| | `i` / `a` / `Enter` | type a message |
| | `e` | write the message in `$EDITOR` (nvim) |
| | `h` / `q` / `Esc` | back to the list |
| anywhere | `Ctrl+h` / `Ctrl+l` | Chats / Messages box; at the edge, the tmux pane next door |
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

### Message menu

Select a message with `j`/`k`, press `Space`. Move with `Ctrl+n`/`Ctrl+p`, pick with `Enter` (or press the letter), close with `Esc`.

| Option | Key | Notes |
|---|---|---|
| Reply | `r` | the quote shows in the Compose title; `Esc` (in normal mode) cancels |
| Edit | `e` | your own text messages, first 15 minutes |
| React | `+` | 👍 ❤️ 😂 😮 😢 🙏 🔥 🎉, or remove yours |
| Show / Hide image | `v` | shows pictures that are hidden (older than the newest 40, or `images=off`/`thumbnail`) |
| Open image in viewer | `o` | opens the downloaded picture with `xdg-open` |
| Copy text | `y` | via `xclip` |
| Forward | `f` | type to search a chat, `Enter` to send |
| Star / Unstar | `s` | ★ shows next to the message |
| Pin | `p` | 24 hours, 7 days or 30 days, or unpin |
| Delete for everyone | `d` | your own messages, first 2 days; asks first |

Commands: `:pin` `:unpin` `:mute` `:unmute` `:archive` `:unarchive` `:archived` `:read` `:q`

## Settings

`~/.config/watui/config` is created on first run:

```
images = always        # always | thumbnail | off
image_width = 40       # biggest picture size, in terminal cells
image_height = 15
```

| `images` | What you see |
|---|---|
| `always` | full picture inline (downloads it; newest 40 per chat) |
| `thumbnail` | the small blurry preview WhatsApp sends with the message, no download |
| `off` | text only, e.g. `[image] caption` |

Change a setting while watui is running with `:set images=thumbnail` (that only lasts until you quit), or type `:set` to see the current values.

Pictures need kitty. Inside tmux they also need `set -g allow-passthrough on` in your tmux config.

With vim-tmux-navigator, tmux steals `Ctrl+h`/`Ctrl+l` unless watui is in its vim pattern. Add `watui` to the pattern by rebinding `C-h`/`C-l` after the `run tpm` line (see `~/.tmux.conf`).

## Where things are stored

`~/.local/share/watui/`:

| File | What |
|---|---|
| `whatsapp.db` | your link keys (anyone with this file can read your chats, so keep it private) |
| `messages.db` | message history shown in the app |
| `watui.log` | errors (`WATUI_DEBUG=1 watui` logs more) |

Downloaded pictures are cached in `~/.cache/watui/images/` (safe to delete).

## Not supported yet

Sending new images or files, video playback, animated stickers, voice/video calls, desktop notifications, showing which message is pinned.

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
make install    # ~/.local/bin/watui
```
