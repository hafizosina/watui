# watui todo

## Open
- search all chats at once (`/` in the messages pane searches the open chat)
- record and send voice notes
- send a file / picture as a reply (attaching now cancels the reply)
- README screenshots for the new features: multi-select, attachments in Compose, large Compose (`make screenshots`)

## Done
- [x] search the open chat: `/` in the messages pane, `n` / `N`, highlighted matches, whole history
- [x] proper releases: v0.1.0, MIT license, CHANGELOG, release workflow (`make release V=…`), module path github.com/hafizosina/watui
- [x] macOS port (builds and passes tests on macOS in CI) — 1843606
- [x] calls show in the chat (answered / declined / missed) + ringing banner, ctrl+x decline — 50060a3
- [x] send emoji (`:emoji` popup) and stickers (ctrl+t) — 50060a3
- [x] voice note pause (leader o) froze watui — fixed in 50060a3
- [x] own notification sound, different from other apps (`notify_sound = watui`) — 50060a3
- [x] README screenshots of the features — 50060a3, 5326aa9
- [x] govim profile (like cvim, jvim, gdvim) — ~/.config/govim
- [x] open archived chats (Archived (N) row, `A`) — 50060a3
- [x] search chats by name (`/`) — works, covered by a test
- [x] open links from a message in the browser (message menu `l`) — 59af365
- [x] select several messages (`v` / `V`) and copy / forward / star / save / delete them — 59af365
- [x] send files and pictures: `p` paste (clipboard picture, copied files, paths), `u` / `:upload` file dialog — 204accd
- [x] Compose shares the system clipboard (yank → other apps, `p` pastes the latest copy) — 204accd
- [x] large Compose (ctrl+g / `:big`), fills from the bottom — 204accd
- [x] open image in viewer opens the original, not the thumbnail — 204accd
- [x] Profile box same height as the chat header — 204accd
