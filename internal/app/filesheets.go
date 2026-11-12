package app

import (
	"time"

	"github.com/Licoy/sshyard/internal/layout"
	"github.com/Licoy/sshyard/internal/theme"
	"github.com/Licoy/sshyard/locales"
	"github.com/egoist/mygo/ui"
)

// fileDialogKey identifies a window overlay, whose parent is shared by
// every pane even when the dialog is requested from inside a pane.
type fileDialogKey struct {
	pane int
	kind string
}

type fileDialogTrace struct {
	open, painted bool
	bounds        ui.Rect
	revision      uint64
}

// fileSheetsView builds files dialogs after the workspace, its virtual
// lists, rail and status bar have finished building. Only panes of the
// visible tab participate, matching the lifetime of their original views.
func (a *App) fileSheetsView(c *ui.Context, tok theme.Tokens, m locales.Messages) {
	if a.page != pageWorkspace || a.ws == nil {
		return
	}
	t := a.ws.ActiveTab()
	if t == nil {
		return
	}
	for _, it := range t.Items() {
		cn := a.conns[it.ID]
		if cn == nil || cn.kind != layout.Files || cn.fs == nil {
			continue
		}
		a.askView(c, cn, m)
		a.deleteView(c, cn, m)
		a.conflictView(c, cn, m)
		a.openAskView(c, cn, m)
		a.withView(c, cn, tok, m)
		a.snippetView(c, cn, tok, m)
		a.transfersSheet(c, cn, tok, m)
		a.infoView(c, cn, tok, m)
		a.compressView(c, cn, tok, m)
		a.previewView(c, cn, tok, m)
		a.fileFavoritesView(c, cn, tok, m)
	}
}

// fileModal gives each pane's dialog its own identity in the window's
// overlay layer. A closed modal must not leave a pending Context.Key for
// whichever control is built next.
func (a *App) fileModal(c *ui.Context, cn *conn, kind string, open *bool, build func()) ui.Element {
	var panel ui.Element
	if *open {
		panel = ui.Modal(c.Key(fileDialogKey{cn.id, kind}), open, build)
	}
	if cn.fs.dialogTrace == nil {
		cn.fs.dialogTrace = map[string]*fileDialogTrace{}
	}
	d := cn.fs.dialogTrace[kind]
	if d == nil {
		d = &fileDialogTrace{}
		cn.fs.dialogTrace[kind] = d
	}
	if d.open != *open {
		d.open, d.painted = *open, false
		a.diagnostic("files.dialog.state", map[string]any{"pane": cn.id, "kind": kind, "open": *open, "frame": a.uiTrace.frame, "revision": a.uiTrace.revision})
		if *open {
			// Revisit the final geometry even when the pointer stops on
			// the trigger immediately after it opened the dialog.
			c.After(time.Second / 60)
		}
	}
	if a.p.Diagnostic == nil {
		return panel
	}
	return panel.DrawOver(func(_ *ui.Painter, r ui.Rect) {
		if d.painted && d.bounds == r && d.revision == a.uiTrace.revision {
			return
		}
		d.painted, d.bounds, d.revision = true, r, a.uiTrace.revision
		a.diagnostic("files.dialog.paint", map[string]any{"pane": cn.id, "kind": kind, "bounds": r, "frame": a.uiTrace.frame, "revision": a.uiTrace.revision})
	})
}
