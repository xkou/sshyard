package app

import (
	"github.com/Licoy/sshyard/internal/layout"
	"github.com/Licoy/sshyard/internal/sftpsvc"
	"github.com/Licoy/sshyard/locales"
	"github.com/egoist/mygo/ui"
)

// remoteListed commits a successful listing and remembers it for the next
// files pane. Failed listings, editor trees and closed panes never replace
// a saved host's last folder. The history worker handles disk I/O.
func (a *App) remoteListed(s *remoteSide, dir string, entries []sftpsvc.Entry, unfinished []sftpsvc.Unfinished) {
	s.unfinished, s.restoringDir = unfinished, false
	s.show(dir, entries)
	if a.fileDirs == nil || a.store == nil {
		return
	}
	for _, cn := range a.conns {
		if cn.kind != layout.Files || cn.fs == nil || &cn.fs.remote != s {
			continue
		}
		a.rememberRemoteDirectory(cn)
		return
	}
}

func (a *App) rememberRemoteDirectory(cn *conn) {
	if a.fileDirs == nil || a.store == nil || cn.kind != layout.Files || cn.fs == nil || cn.fs.remote.client == nil {
		return
	}
	if _, saved := a.store.Get(cn.host.ID); saved {
		a.fileDirs.Remember(cn.host.ID, cn.fs.remote.dir)
	}
}

func (a *App) fileLocationError(phase string, err error) {
	a.diagnostic("files.location_error", map[string]any{"phase": phase})
	a.post(func() {
		a.toast(func(m locales.Messages) ui.Toast {
			return ui.Toast{Title: m.ErrGeneric(securityErrorText(m, err)), Type: "error"}
		})
	})
}
