# Changelog

All notable changes to watui. Versions follow [Semantic Versioning](https://semver.org): while the version starts with `0.`, keys and settings may still change between minor versions.

## [Unreleased]

### Added
- Drafts: text left in Compose when you switch chats is kept per chat (with @mentions and the reply you started) and comes back when you return, also after a restart. The chat list shows it as `Draft: …`.
- Separate notification sounds: direct messages and @mentions / replies to you play a new `sword` sound, other group messages the old chime. Each kind can be changed with `notify_sound_personal`, `notify_sound_mention` and `notify_sound_group` (built-in `sword`, `chime`, `soft`, `ping`, or `system`, `off`, a file).
- Download progress on the attachment itself: while a file, video, voice note or picture downloads, a bar with the percentage replaces its `space → Open` / `Play` hint (no message at the bottom).

### Changed
- Deleting in Compose (`dd`, `x`, `dw`, `cw`, visual `d`, …) no longer copies the text: it doesn't touch the clipboard, and `p` doesn't paste it back. Only yanks copy.
- Holding or tapping `J` / `K` quickly runs through the chat list without loading every chat on the way: the chat opens when you stop (150 ms after the last press). Any other key opens it right away.

### Fixed
- In tmux, a picture that arrived while watui's pane was hidden showed as an empty block (tmux drops kitty graphics for hidden panes with `allow-passthrough on`). watui now re-sends the open chat's pictures when its pane gets focus back; the README now recommends `allow-passthrough all`.
- Opening, playing or saving a large file no longer fails after 60 seconds: a download now only stops when nothing arrives for a minute. Downloads stream to disk instead of being held in memory.

## [v0.1.1] - 2026-11-17

### Added
- Search the open chat: `/` in the messages pane searches its whole history; `Enter` jumps to the nearest match above, `n` / `N` older / newer, matches highlighted, the title shows "3 of 12"; `Esc` clears.

### Fixed
- `dd`, `x` and other deletes or yanks in Compose froze watui on Linux (copying to the clipboard waited for xclip's background process); `y` Copy text in the message menu had the same wait in the background. Introduced in v0.1.0.
- A message marked "✗ failed" (the send timed out) now shows ✓✓ once it turns out to have arrived.
- Messages sent within the same second keep their order after a history sync; the chat list preview matches the last message.
- `:set theme=…` and `:set notify_sound=…` take effect right away instead of after a restart; `:set` lists both.
- A blank `$EDITOR` no longer crashes watui (falls back to nvim).

## [v0.1.0] - 2026-11-17

First release.

### Chats and messages
- Vim-style keys in lazygit-style boxes: profile, chats, chat header, messages, Compose.
- Chat list with search (`/`), archived chats, pinned and muted chats, unread and @mention badges; chat menu to pin, mute, archive, mark read/unread, leave or delete.
- Messages with replies (jump to the replied message and back), reactions, edits, forwarded and starred marks, long messages folded; deleted messages kept and marked.
- Mentions shown as names; `@` in Compose mentions group members.
- Message menu: reply, edit, react, forward, star, pin, delete for everyone, copy, open links, save media, message info (delivered / read by whom).
- Select several messages (`v` / `V`) to copy, forward, star, save or delete them at once.
- New chat and new group; contact and group details with profile pictures.

### Media and files
- Pictures, stickers and GIFs drawn inline in kitty (animated ones move); videos and voice notes play with mpv.
- Send files and pictures: paste a copied picture or files (`p`, `Ctrl+V`), or pick them in a file dialog (`u`, `:upload`), with an optional caption.
- Stickers picker (`Ctrl+T`) and `:emoji` completion.

### Compose
- A small vim: normal, insert, visual modes, operators, undo/redo.
- Shares the system clipboard, like vim's `unnamedplus`.
- Large mode (`Ctrl+G`, `:big`) for long messages; `$EDITOR` with `e` / `Ctrl+E`.

### Everything else
- Several WhatsApp accounts (profiles), all connected at once.
- Desktop notifications with their own sound, per-chat replace, click to open; calls logged in the chat, ringing banner with decline.
- Seven themes plus your own; command line with suggestions.
- Only one watui runs at a time, with an offer to close the other copy.
- Runs on Linux; macOS builds and passes tests (new, not yet tried by hand); Windows through WSL2.

### Security
- Text from WhatsApp is cleaned before display (no terminal escape sequences).
- Only a message's author can edit it; the author or a group admin can delete it.

[Unreleased]: https://github.com/hafizosina/watui/compare/v0.1.1...HEAD
[v0.1.1]: https://github.com/hafizosina/watui/compare/v0.1.0...v0.1.1
[v0.1.0]: https://github.com/hafizosina/watui/releases/tag/v0.1.0
