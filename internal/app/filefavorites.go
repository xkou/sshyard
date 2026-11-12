package app

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/Licoy/sshyard/internal/icons"
	"github.com/Licoy/sshyard/internal/sftpsvc"
	"github.com/Licoy/sshyard/internal/theme"
	"github.com/Licoy/sshyard/locales"
	"github.com/egoist/mygo/ui"
)

// fileFavoritesForm keeps the editable path and the chosen saved folder.
// Opening a folder runs off the UI thread and closes the sheet only when
// its listing succeeds.
type fileFavoritesForm struct {
	remote   bool
	path     string
	selected int
	list     ui.ListState
	hold     focusHold
	err      string
	busy     bool
	cancel   context.CancelFunc
}

// fileFavoriteHost gives this computer one list shared by all local sides,
// and each remote host its own list shared by that host's files panes.
func fileFavoriteHost(cn *conn, remote bool) string {
	if !remote {
		return localID
	}
	return cn.host.ID
}

func (a *App) fileFavoritePaths(cn *conn, remote bool) []string {
	return a.settings.FileFavorites[fileFavoriteHost(cn, remote)]
}

// setFileFavoritePaths replaces only this host's list, keeping the old
// settings intact if the atomic settings write fails.
func (a *App) setFileFavoritePaths(cn *conn, remote bool, paths []string) error {
	s := a.settings
	s.FileFavorites = maps.Clone(s.FileFavorites)
	if s.FileFavorites == nil {
		s.FileFavorites = map[string][]string{}
	}
	key := fileFavoriteHost(cn, remote)
	if len(paths) == 0 {
		delete(s.FileFavorites, key)
	} else {
		s.FileFavorites[key] = slices.Clone(paths)
	}
	return a.ApplySettings(s)
}

func (a *App) openFileFavorites(cn *conn, remote bool) {
	fs := cn.fs
	if fs.favorites != nil && fs.favorites.cancel != nil {
		fs.favorites.cancel()
	}
	f := &fileFavoritesForm{remote: remote, path: fs.side(remote).dir, selected: -1}
	f.list.Selected = &f.selected
	fs.favorites, fs.favoritesOpen = f, true
	a.menu = dropMenu{}
}

func (a *App) addFileFavorite(cn *conn, f *fileFavoritesForm, m locales.Messages) {
	if f.busy {
		return
	}
	home := a.localHome()
	if f.remote {
		home = cn.fs.remote.home
	}
	dir := typedPath(f.path, cn.fs.side(f.remote).dir, home, f.remote)
	if dir == "" || strings.ContainsRune(dir, 0) {
		f.err = m.SecuritySftpInvalidPath()
		return
	}
	paths := a.fileFavoritePaths(cn, f.remote)
	if slices.Contains(paths, dir) {
		f.err = m.FileFavoriteExists()
		return
	}
	if err := a.setFileFavoritePaths(cn, f.remote, append(slices.Clone(paths), dir)); err != nil {
		f.err = m.ErrGeneric(securityErrorText(m, err))
		return
	}
	f.err, f.selected = "", len(paths)
}

func (a *App) removeFileFavorite(cn *conn, f *fileFavoritesForm, dir string, m locales.Messages) {
	if f.busy {
		return
	}
	paths := slices.DeleteFunc(slices.Clone(a.fileFavoritePaths(cn, f.remote)), func(p string) bool { return p == dir })
	if err := a.setFileFavoritePaths(cn, f.remote, paths); err != nil {
		f.err = m.ErrGeneric(securityErrorText(m, err))
		return
	}
	f.err, f.selected = "", -1
}

// goToFileFavorite obtains the new listing before changing the browser.
// Missing or unreadable folders leave both the sheet and the current
// directory intact. A closed sheet, replaced pane or reconnected client
// cannot apply a late result.
func (a *App) goToFileFavorite(cn *conn, f *fileFavoritesForm, dir string, m locales.Messages) {
	fs := cn.fs
	if f.busy || fs.favorites != f || !fs.favoritesOpen {
		return
	}
	sc, connects := fs.remote.client, fs.remote.connects
	if f.remote && sc == nil {
		f.err = m.FileFavoriteOpenFailed(shownName(dir), m.StateDisconnected())
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), openTimeout)
	f.busy, f.err, f.cancel = true, "", cancel
	go func() {
		defer cancel()
		var entries []sftpsvc.Entry
		var unfinished []sftpsvc.Unfinished
		var err error
		if f.remote {
			entries, unfinished, err = sc.ListUnfinishedContext(ctx, dir)
		} else {
			entries, err = sftpsvc.ListLocal(dir)
			if ctx.Err() != nil {
				err = ctx.Err()
			}
		}
		a.post(func() {
			if a.conns[cn.id] != cn || cn.fs != fs || fs.favorites != f || !fs.favoritesOpen {
				return
			}
			f.busy, f.cancel = false, nil
			if f.remote && (fs.remote.client != sc || fs.remote.connects != connects) {
				f.err = m.FileFavoriteOpenFailed(shownName(dir), m.StateDisconnected())
				return
			}
			if err != nil {
				f.err = m.FileFavoriteOpenFailed(shownName(dir), securityErrorText(m, err))
				return
			}
			if f.remote {
				s := &fs.remote
				if s.listErr {
					s.err, s.listErr = "", false
				}
				s.listSeq++
				a.remoteListed(s, dir, entries, unfinished)
			} else {
				l := &fs.local
				if l.listErr {
					l.err, l.listErr = "", false
				}
				l.loaded = true
				l.show(dir, entries)
				a.readLocalNames(l, entries)
			}
			fs.side(f.remote).refocus = true
			fs.favoritesOpen = false
		})
	}()
}

func (a *App) fileFavoritesView(c *ui.Context, cn *conn, tok theme.Tokens, m locales.Messages) {
	fs := cn.fs
	f := fs.favorites
	if f == nil {
		return
	}
	paths := a.fileFavoritePaths(cn, f.remote)
	f.list.Key = func(i int) any { return paths[i] }
	f.list.Label = func(i int) string { return shownName(paths[i]) }
	title, host := m.LocalFileFavorites(), m.Local()
	if f.remote {
		title, host = m.RemoteFileFavorites(), a.pv(a.liveHost(cn.host).Label())
	}
	sheet(a.fileModal(c, cn, "filefavorites", &fs.favoritesOpen, func() {
		ui.Column(c).Width(theme.Space(140)).Gap(theme.Space(3)).Children(func() {
			ui.Row(c).Gap(theme.Space(2)).AlignItems(ui.Center).Children(func() {
				softTile(c, icons.Star, tok.Accent, theme.Space(8))
				ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(theme.Space(0.5)).Children(func() {
					sheetTitle(c, title)
					ui.Text(c, host).FontSize(theme.Footnote).TextColor(tok.TextMuted).SingleLine()
				})
			})
			add := false
			ui.Row(c).FillWidth().Gap(theme.Space(2)).AlignItems(ui.Center).Children(func() {
				in := formTextInput(c.Key("path"), &f.path).Label(m.CrumbsLabel()).Placeholder(m.PathPlaceholder()).AutoFocus().Grow(1).Shrink(1).MinWidth(0).Disabled(f.busy)
				f.hold.apply(c, f, in)
				add = in.Submitted()
				if pill(c, m.AddMenu(), true, false, f.busy || strings.TrimSpace(f.path) == "") {
					add = true
				}
			})
			if add {
				a.addFileFavorite(cn, f, m)
			}
			ui.Text(c, m.FileFavoritesNote()).FontSize(theme.Footnote).TextColor(tok.TextMuted)
			ui.Column(c).Height(theme.Space(72)).FillWidth().MinWidth(0).Padding(theme.Space(1)).Radius(theme.RadiusCard).Background(tok.Fill).Children(func() {
				if len(paths) == 0 {
					ui.Column(c).Fill().Center().Children(func() {
						ui.Text(c, m.FileFavoritesEmpty()).FontSize(theme.Footnote).TextColor(tok.TextMuted)
					})
					return
				}
				list := ui.List(c.Key("favorites"), &f.list, len(paths), func(i int) {
					dir := paths[i]
					ui.Row(c).MinHeight(theme.Space(8)).Padding(theme.Space(1), theme.Space(2)).Gap(theme.Space(2)).FillWidth().MinWidth(0).AlignItems(ui.Center).Children(func() {
						fileImage(c, entryIcon(sftpsvc.Entry{Name: sideBase(f.remote, dir), IsDir: true}), theme.Space(4))
						ui.Text(c, shownName(dir)).SingleLine().Grow(1).Shrink(1).MinWidth(0)
						if (IconButton{Icon: icons.Close, Label: m.RemoveFavorite(), Disabled: f.busy, Size: Small}).Clicked(c, "remove") {
							a.removeFileFavorite(cn, f, dir, m)
						}
					})
				}).Fill().Disabled(f.busy)
				if list.Submitted() && f.selected >= 0 && f.selected < len(paths) {
					a.goToFileFavorite(cn, f, paths[f.selected], m)
				}
			})
			if f.err != "" {
				ui.Text(c, f.err).FontSize(theme.Footnote).TextColor(tok.Danger)
			}
			ui.Row(c).FillWidth().Gap(theme.Space(2)).AlignItems(ui.Center).Children(func() {
				if f.busy {
					ui.Spinner(c)
					ui.Text(c, m.Loading()).FontSize(theme.Footnote).TextColor(tok.TextMuted)
				}
				ui.Spacer(c)
				if pill(c, m.Close(), false, false, false) {
					fs.favoritesOpen = false
				}
			})
		})
	}), tok)
	if !fs.favoritesOpen && fs.favorites == f {
		if f.cancel != nil {
			f.cancel()
		}
		fs.favorites = nil
	}
}
