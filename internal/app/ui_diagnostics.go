package app

import (
	"time"

	"github.com/egoist/mygo/ui"
)

type uiDiagnosticState struct {
	size        [2]float32
	revision    uint64
	frame       uint64
	frameAt     time.Time
	lastReport  time.Time
	context     *ui.Context
	viewActive  bool
	viewStarted time.Time
}

type fileTableDiagnostics struct {
	bounds                       ui.Rect
	columns, entries, chosen, at int
	last                         time.Time
}

func (a *App) diagnostic(event string, fields map[string]any) {
	if a.p.Diagnostic != nil {
		a.p.Diagnostic(event, fields)
	}
}

func (a *App) observeUI(c *ui.Context) {
	d := &a.uiTrace
	w, h := c.Size()
	if size := [2]float32{w, h}; size != d.size {
		d.size, d.revision = size, d.revision+1
		a.diagnostic("ui.resize", map[string]any{"width": w, "height": h, "revision": d.revision, "maximized": a.maximized, "zoom": a.zoom.now})
	}
	if d.frameAt == c.Now() {
		return
	}
	d.frameAt, d.frame = c.Now(), d.frame+1
	if a.p.Diagnostic == nil || !d.lastReport.IsZero() && c.Now().Sub(d.lastReport) < 10*time.Second {
		return
	}
	d.lastReport = c.Now()
	files, favorites, sheets := 0, 0, 0
	for _, cn := range a.conns {
		if cn.fs != nil {
			files++
			if cn.fs.favoritesOpen {
				favorites++
			}
			fs := cn.fs
			for _, open := range []bool{fs.favoritesOpen, fs.askOpen, fs.delOpen, fs.conflictOpen, fs.openOpen, fs.withOpen, fs.compressOpen, fs.infoOpen, fs.previewOpen, fs.queueOpen, fs.snippetOpen} {
				if open {
					sheets++
				}
			}
		}
	}
	a.diagnostic("ui.frame", map[string]any{"frame": d.frame, "revision": d.revision, "width": w, "height": h, "maximized": a.maximized, "files": files, "favorites_open": favorites, "file_sheets": sheets, "menu_open": a.menu.key != "", "system_title_bar": a.systemTitleBar})
}

func (a *App) traceFileTable(c *ui.Context, cn *conn, remote bool, columns int) {
	if a.p.Diagnostic == nil {
		return
	}
	sd := cn.fs.side(remote)
	d := &sd.trace
	bounds := sd.list.Handle.Bounds(c)
	changed := d.bounds != bounds || d.columns != columns || d.entries != len(sd.entries) || d.chosen != sd.chosen.Len() || d.at != sd.sel
	if !d.last.IsZero() && (!changed || c.Now().Sub(d.last) < 100*time.Millisecond) {
		return
	}
	first, last := sd.list.Visible()
	*d = fileTableDiagnostics{bounds: bounds, columns: columns, entries: len(sd.entries), chosen: sd.chosen.Len(), at: sd.sel, last: c.Now()}
	a.diagnostic("files.geometry", map[string]any{"pane": cn.id, "remote": remote, "frame": a.uiTrace.frame, "revision": a.uiTrace.revision, "generation": sd.generation, "bounds": bounds, "allocated_width": sd.width, "columns": columns, "entries": len(sd.entries), "chosen": sd.chosen.Len(), "at": sd.sel, "first": first, "last": last, "focused": sd.list.FocusWithin(c), "marquee": sd.marquee.active})
}

func (a *App) traceFilePointer(c *ui.Context, cn *conn, remote bool, target string, row int, ev ui.InputEvent) {
	if a.p.Diagnostic == nil || ev.Kind != ui.InputPointerDown && ev.Kind != ui.InputPointerUp {
		return
	}
	sd := cn.fs.side(remote)
	a.diagnostic("files.pointer", map[string]any{"pane": cn.id, "remote": remote, "target": target, "row": row, "kind": int(ev.Kind), "button": ev.Button, "clicks": ev.Clicks, "mods": int(ev.Mods), "x": ev.X, "y": ev.Y, "bounds": sd.list.Handle.Bounds(c), "chosen": sd.chosen.Len(), "at": sd.sel, "revision": a.uiTrace.revision, "generation": sd.generation, "focused": sd.list.Handle.FocusWithin(c)})
}
