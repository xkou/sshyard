package app

import (
	"time"

	"github.com/Licoy/sshyard/internal/theme"
	"github.com/egoist/mygo/ui"
)

// fileMarquee captures a drag starting in the table's background. Names
// keep their own drag source, so dragging selected files still transfers.
type fileMarquee struct {
	pending, active bool
	fromX, fromY    float32
	x, y            float32
	anchor, end     int
	offset          float32
	base            []string
	prior           int
	nextScroll      time.Time
}

// marqueeRow finds the row at y from painted row boxes. The indexes
// just outside the entries represent the empty space before or after them.
func (sd *sideList) marqueeRow(y, pad float32) (index int, center float32) {
	index, distance := len(sd.entries), float32(1e30)
	first, last := len(sd.entries), -1
	var top, bottom float32
	for _, row := range sd.rows {
		b := row.bounds
		if b.H <= 0 {
			continue
		}
		if row.index < first {
			first, top = row.index, b.Y
		}
		if row.index > last {
			last, bottom = row.index, b.Y+b.H
		}
		cy := b.Y + b.H/2
		d := max(y-cy, cy-y)
		if d < distance {
			index, center, distance = row.index, cy, d
		}
	}
	if first == 0 && y < top-pad {
		return -1, 0
	}
	if last == len(sd.entries)-1 && y > bottom+pad {
		return len(sd.entries), 0
	}
	return index, center
}

func (sd *sideList) updateMarquee(b ui.Rect, header float32) bool {
	m := &sd.marquee
	if !m.pending {
		return false
	}
	dx, dy := m.x-m.fromX, m.y-m.fromY
	if !m.active && dx*dx+dy*dy < theme.Space(1)*theme.Space(1) {
		return false
	}
	m.active = true
	y := b.Y + min(max(m.y, header), max(header, b.H-1))
	to, _ := sd.marqueeRow(y, (header-1)*1.5/8)
	if m.end == to {
		return false
	}
	m.end = to
	sd.chosen.Clear()
	for _, name := range m.base {
		sd.chosen.Add(name)
	}
	lo, hi := max(min(m.anchor, to), 0), min(max(m.anchor, to), len(sd.entries)-1)
	for i := lo; i <= hi; i++ {
		sd.chosen.Add(sd.entries[i].Name)
	}
	sd.sel = -1
	if lo <= hi {
		sd.sel = min(max(to, lo), hi)
	} else if m.prior >= 0 && m.prior < len(sd.entries) && sd.chosen.Has(sd.entries[m.prior].Name) {
		sd.sel = m.prior
	}
	return true
}

func (sd *sideList) marqueeRect(b ui.Rect, header float32) ui.Rect {
	m := &sd.marquee
	y := m.fromY
	if m.anchor >= 0 && m.anchor < len(sd.entries) {
		found, first := false, len(sd.entries)
		for _, row := range sd.rows {
			r := row.bounds
			if r.H <= 0 {
				continue
			}
			first = min(first, row.index)
			if row.index == m.anchor {
				y, found = r.Y+r.H/2-b.Y+m.offset, true
			}
		}
		if !found {
			y = b.H
			if m.anchor < first {
				y = header
			}
		}
	}
	x0, x1 := max(0, min(m.fromX, m.x)), min(b.W, max(m.fromX, m.x))
	y0, y1 := max(header, min(y, m.y)), min(b.H, max(y, m.y))
	return ui.Rect{X: x0, Y: y0, W: max(0, x1-x0), H: max(0, y1-y0)}
}

func (a *App) fileMarqueeView(c *ui.Context, cn *conn, remote bool, tb ui.Element, header float32, tok theme.Tokens, menu func(float32, float32)) {
	sd := cn.fs.side(remote)
	if sd.marquee.pending && tb.Shortcut(0, ui.KeyEscape) {
		sd.marquee = fileMarquee{}
		sd.only(-1)
	}
	tb.HandleInput(func(ev ui.InputEvent) bool {
		a.traceFilePointer(c, cn, remote, "table", -1, ev)
		b := sd.list.Handle.Bounds(c)
		m := &sd.marquee
		if ev.Kind == ui.InputPointerDown && ev.Button == 1 {
			*m = fileMarquee{}
			menu(b.X+ev.X, b.Y+ev.Y)
			return true
		}
		switch ev.Kind {
		case ui.InputPointerDown:
			if ev.Button != 0 || ev.X < 0 || ev.X >= b.W || ev.Y < header || ev.Y >= b.H {
				return false // column headers and their resize handles
			}
			row, center := sd.marqueeRow(b.Y+ev.Y, (header-1)*1.5/8)
			if row >= 0 && row < len(sd.entries) && ev.Mods&(toggleMods|ui.Shift) != 0 {
				return false // keep the table's modified row clicks
			}
			sd.list.Handle.Focus(c)
			if ev.Clicks == 2 && row >= 0 && row < len(sd.entries) {
				sd.only(row)
				sd.pendingOpen = sd.entries[row].Name
				return true
			}
			*m = fileMarquee{pending: true, fromX: ev.X, fromY: ev.Y, x: ev.X, y: ev.Y, anchor: row, end: -2, offset: b.Y + ev.Y - center, prior: sd.sel}
			if ev.Mods&(toggleMods|ui.Shift) != 0 {
				for name := range sd.chosen.All() {
					m.base = append(m.base, name)
				}
			}
			a.diagnostic("files.marquee.begin", map[string]any{"pane": cn.id, "remote": remote, "row": row, "chosen": sd.chosen.Len(), "revision": a.uiTrace.revision})
			return true
		case ui.InputPointerMove:
			if !m.pending || ev.Button != 0 {
				return false
			}
			m.x, m.y = ev.X, ev.Y
			sd.updateMarquee(b, header)
			return true
		case ui.InputPointerUp:
			if !m.pending || ev.Button != 0 {
				return false
			}
			m.x, m.y = ev.X, ev.Y
			sd.updateMarquee(b, header)
			if !m.active {
				if m.anchor >= 0 && m.anchor < len(sd.entries) {
					sd.only(m.anchor)
				} else if len(m.base) == 0 {
					sd.only(-1)
				}
			}
			a.diagnostic("files.marquee.end", map[string]any{"pane": cn.id, "remote": remote, "dragged": m.active, "chosen": sd.chosen.Len(), "at": sd.sel, "revision": a.uiTrace.revision})
			*m = fileMarquee{}
			return true
		}
		return false
	})
	m := &sd.marquee
	if !m.pending || !m.active {
		return
	}
	b := sd.list.Handle.Bounds(c)
	if sd.updateMarquee(b, header) {
		c.Invalidate()
	}
	// Scroll by rows, so virtualized entries beyond the viewport can join
	// the selection while the pointer remains near an edge.
	if m.x >= 0 && m.x <= b.W {
		first, last := sd.list.Visible()
		edge, scroll := theme.Space(4), 0
		if m.y < header+edge && first > 0 {
			scroll = -1
		} else if m.y > b.H-edge && last < len(sd.entries)-1 {
			scroll = 1
		}
		if scroll != 0 {
			if !c.Now().Before(m.nextScroll) {
				if scroll < 0 {
					sd.list.ScrollTo(first-1, ui.Start)
				} else {
					sd.list.ScrollTo(last+1, ui.End)
				}
				m.nextScroll = c.Now().Add(40 * time.Millisecond)
			}
			c.After(max(time.Millisecond, m.nextScroll.Sub(c.Now())))
		}
	}
	r := sd.marqueeRect(b, header)
	if r.W > 0 && r.H > 0 {
		tb.Children(func() {
			ui.Box(c.Key("marquee")).Absolute().Left(r.X).Top(r.Y).Size(r.W, r.H).Background(tok.Accent.Alpha(0.12)).Border(1, tok.Accent.Alpha(0.7)).Radius(theme.RadiusSmall).IgnoreInput(true).Role(ui.RoleNone)
		})
	}
}
