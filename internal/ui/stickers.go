package ui

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hafizosina/watui/internal/media"
	"github.com/hafizosina/watui/internal/wa"
)

const (
	stickerCols   = 12
	stickerRows   = 6
	stickerLimit  = 40
	stickerGapCol = 2
)

type stickerPicker struct {
	refs []wa.StickerRef
	sel  int
}

func (m *Model) openStickers() {
	if m.openJID == "" {
		m.setFlash("open a chat first", true)
		return
	}
	refs, err := m.wa.RecentStickers(stickerLimit)
	if err != nil {
		m.setFlash(err.Error(), true)
		return
	}
	if len(refs) == 0 {
		m.setFlash("no stickers yet — stickers you receive or send show up here", false)
		return
	}
	if m.images != nil {
		for _, r := range refs {
			m.images.Request(r.ChatJID, r.ID, media.Full, media.Box{MaxCols: stickerCols, MaxRows: stickerRows})
		}
	}
	m.stickers = &stickerPicker{refs: refs}
}

func (m Model) stickerPerRow() int {
	return max(1, min(8, (m.width-8)/(stickerCols+stickerGapCol)))
}

func (m Model) stickerKey(key string) (tea.Model, tea.Cmd) {
	sp := m.stickers
	per := m.stickerPerRow()
	n := len(sp.refs)
	switch key {
	case "esc", "q", "ctrl+t", "ctrl+c":
		m.stickers = nil
	case "l", "right", "ctrl+n", "tab":
		sp.sel = (sp.sel + 1) % n
	case "h", "left", "ctrl+p", "shift+tab":
		sp.sel = (sp.sel - 1 + n) % n
	case "j", "down":
		sp.sel = min(n-1, sp.sel+per)
	case "k", "up":
		sp.sel = max(0, sp.sel-per)
	case "enter", " ":
		ref, chat := sp.refs[sp.sel], m.openJID
		m.stickers = nil
		client := m.wa
		return m, m.actionCmd("sticker sent", func(ctx context.Context) error { return client.SendSticker(ctx, chat, ref) })
	}
	return m, nil
}

func (m Model) stickerView() (string, int, int) {
	sp := m.stickers
	per := m.stickerPerRow()
	cellW := stickerCols + stickerGapCol
	w := per*cellW + 4
	rowsTotal := (len(sp.refs) + per - 1) / per
	visibleRows := max(1, min(rowsTotal, (m.mainH()-6)/(stickerRows+2)))
	selRow := sp.sel / per
	first := max(0, selRow-visibleRows+1)

	var lines []string
	for r := first; r < first+visibleRows && r < rowsTotal; r++ {
		block := make([]string, stickerRows+1)
		for c := 0; c < per; c++ {
			i := r*per + c
			cell := make([]string, stickerRows)
			if i < len(sp.refs) && m.images != nil {
				img, status, _ := m.images.Lines(sp.refs[i].ID, media.Full)
				switch {
				case len(img) > 0:
					top := (stickerRows - len(img)) / 2
					for y, l := range img {
						if top+y < stickerRows {
							cell[top+y] = l
						}
					}
				case status != "":
					cell[stickerRows/2] = styleDim.Render(" " + strings.SplitN(status, ":", 2)[0])
				}
			}
			mark := strings.Repeat(" ", stickerCols)
			if i == sp.sel {
				mark = lipgloss.NewStyle().Foreground(colGreen).Render(strings.Repeat("▔", stickerCols))
			}
			for y := 0; y < stickerRows; y++ {
				block[y] += cell[y] + strings.Repeat(" ", max(0, cellW-lipgloss.Width(cell[y])))
			}
			if i < len(sp.refs) {
				block[stickerRows] += mark + strings.Repeat(" ", stickerGapCol)
			}
		}
		for _, b := range block {
			lines = append(lines, " "+b)
		}
	}
	h := len(lines) + 2
	footer := "hjkl / ^n^p move · enter send · esc close"
	return panel("Stickers · recent", footer, strings.Join(lines, "\n"), w, h, true), w, h
}
