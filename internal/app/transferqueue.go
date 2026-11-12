package app

import (
	"maps"
	"slices"
	"strconv"
	"time"

	"github.com/Licoy/sshyard/internal/icons"
	"github.com/Licoy/sshyard/internal/sftpsvc"
	"github.com/Licoy/sshyard/internal/theme"
	"github.com/Licoy/sshyard/locales"
	"github.com/egoist/mygo/ui"
)

// transferQueueState is the bottom queue opened from the status bar. Its
// snapshot covers every pane, so switching tabs does not hide transfers.
// The owning files panes keep their full history and transfer lifecycle.
type transferQueueState struct {
	open   bool
	list   ui.ListState
	items  []queueTransfer
	queues []*sftpsvc.Transfers
	paused int
	speed  float64
}

type queueTransfer struct {
	owner *conn
	queue *sftpsvc.Transfers
	value sftpsvc.Transfer
}

type queueTransferKey struct{ pane, task int }

// syncTransferQueue takes one snapshot for both the status button and the
// list. Paused tasks remain available to resume; terminal states leave the
// progress list without clearing their original queue's history.
func (a *App) syncTransferQueue(c *ui.Context) {
	q := &a.transferQueue
	var items []queueTransfer
	var queues []*sftpsvc.Transfers
	seen := map[*sftpsvc.Transfers]bool{}
	running, paused, speed := 0, 0, float64(0)
	// Queue IDs are local to a pane. Sort owners and retain each queue's
	// insertion order, so finishing rows cannot reorder other transfers.
	for _, id := range slices.Sorted(maps.Keys(a.conns)) {
		cn := a.conns[id]
		if cn.fs == nil || cn.fs.transfers == nil || seen[cn.fs.transfers] {
			continue
		}
		trs := cn.fs.transfers
		seen[trs] = true
		pending := false
		for _, t := range trs.List() {
			if !t.Active() {
				continue
			}
			items = append(items, queueTransfer{owner: cn, queue: trs, value: t})
			pending = true
			switch t.State {
			case sftpsvc.Running:
				running++
				speed += t.Speed
			case sftpsvc.Paused:
				paused++
			}
		}
		if pending {
			queues = append(queues, trs)
		}
	}
	q.items, q.queues, q.paused, q.speed = items, queues, paused, speed
	if len(items) == 0 {
		q.open = false
		q.list = ui.ListState{}
	} else if running > 0 && q.open && a.page == pageWorkspace {
		c.After(250 * time.Millisecond)
	}
}

func (a *App) transferQueueButton(c *ui.Context, m locales.Messages) {
	q := &a.transferQueue
	if len(q.items) == 0 {
		return
	}
	label := m.ShowTransferQueue()
	if q.open {
		label = m.HideTransferQueue()
	}
	b := IconButton{Icon: icons.Transfers, Label: label, Size: Small, Active: q.open, Toggle: true, Prominent: true, Badge: strconv.Itoa(len(q.items))}
	if b.Show(c, "transferqueue").Margin(0, theme.Space(1), 0, 0).Clicked() {
		q.open = !q.open
		a.invalidate()
	}
}

// transferQueueView expands beneath the workspace and above its status
// bar. It is a native list and keeps the workspace available while open.
func (a *App) transferQueueView(c *ui.Context, m locales.Messages, tok theme.Tokens) {
	q := &a.transferQueue
	items := q.items
	if !q.open || len(items) == 0 {
		return
	}
	q.list.Key = func(i int) any { return queueTransferKey{items[i].owner.id, items[i].value.ID} }
	q.list.Label = func(i int) string {
		t := items[i].value
		return m.TransferProgress(a.pv(shownName(t.Name)), strconv.Itoa(t.Percent()))
	}
	_, windowHeight := c.Size()
	headerHeight, rowHeight := theme.Space(9), theme.Space(12)
	height := min(headerHeight+float32(min(len(items), 4))*rowHeight, windowHeight*0.35)
	a.backgroundSeparator(c, tok.GlassEdge, a.chromeFill(tok.Glass)).Height(theme.Space(0.25)).Shrink(0).FillWidth()
	ui.Column(c.Key("transferqueue")).Height(height).Shrink(0).MinHeight(0).FillWidth().Background(a.contentSurfaceFill(tok.Inset)).Label(m.Transfers()).Children(func() {
		ui.Row(c.Key("header")).Height(headerHeight).Shrink(0).FillWidth().Padding(0, theme.Space(3)).Gap(theme.Space(2)).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icons.Transfers.SVG).Size(Small, Small).Shrink(0).TextColor(tok.Accent)
			ui.Text(c, m.Transfers()).FontSize(theme.Body).FontWeight(600).SingleLine().Shrink(0)
			summary := m.TransferQueuePending(strconv.Itoa(len(items)))
			if q.speed > 0 {
				summary += " · " + humanSize(int64(q.speed)) + "/s"
			}
			ui.Text(c, summary).FontSize(theme.Caption).TextColor(tok.TextMuted).SingleLine().Grow(1).Shrink(1).MinWidth(0)
			if (IconButton{Icon: icons.Pause, Label: m.PauseAll(), Size: Small, Disabled: len(items) == q.paused}).Clicked(c, "pauseall") {
				for _, trs := range q.queues {
					trs.PauseAll()
				}
			}
			if (IconButton{Icon: icons.Resume, Label: m.ResumeAll(), Size: Small, Disabled: q.paused == 0}).Clicked(c, "resumeall") {
				for _, trs := range q.queues {
					trs.ResumeAll()
				}
			}
			if (IconButton{Icon: icons.Close, Label: m.CancelAll(), Size: Small, Danger: true}).Clicked(c, "cancelall") {
				for _, trs := range q.queues {
					trs.CancelAll()
				}
			}
			if (IconButton{Icon: icons.Collapse, Label: m.HideTransferQueue(), Size: Small}).Clicked(c, "hide") {
				q.open = false
				a.invalidate()
			}
		})
		ui.List(c.Key("list"), &q.list, len(items), func(i int) {
			a.transferQueueRow(c, items[i], tok, m)
		}).Grow(1).Shrink(1).MinHeight(0).FillWidth()
	})
}

func (a *App) transferQueueRow(c *ui.Context, item queueTransfer, tok theme.Tokens, m locales.Messages) {
	t, trs := item.value, item.queue
	name := a.pv(shownName(t.Name))
	ui.Row(c.Key(queueTransferKey{item.owner.id, t.ID})).Height(theme.Space(12)).FillWidth().Padding(theme.Space(1), theme.Space(3)).Gap(theme.Space(3)).AlignItems(ui.Center).Children(func() {
		ic := icons.Upload
		if t.Dir == sftpsvc.Download {
			ic = icons.Download
		}
		ui.Icon(c, ic.SVG).Size(Small, Small).Shrink(0).TextColor(tok.TextMuted)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(theme.Space(0.5)).Children(func() {
			ui.Text(c, name).FontSize(theme.Footnote).SingleLine().Tooltip(a.pv(t.Src + " → " + t.Dst))
			detail := a.pv(a.liveHost(item.owner.host).Label())
			if t.Total > 0 {
				detail += " · " + humanSize(t.Bytes) + " / " + humanSize(t.Total)
			}
			ui.Text(c, detail).FontSize(theme.Caption).TextColor(tok.TextMuted).SingleLine()
		})
		ui.Column(c).Width(theme.Space(55)).Shrink(1).MinWidth(0).Gap(theme.Space(1)).Children(func() {
			ui.Progress(c, float64(t.Percent())/100).FillWidth()
			label := ui.Text(c, transferLabel(m, t)).FontSize(theme.Caption).TextColor(tok.TextMuted).SingleLine()
			if t.Err != nil {
				label.Tooltip(a.pv(securityErrorText(m, t.Err)))
			}
		})
		ui.Row(c).Shrink(0).Gap(theme.Space(1)).AlignItems(ui.Center).Children(func() {
			if t.State == sftpsvc.Paused {
				if (IconButton{Icon: icons.Resume, Label: m.ResumeTransfer(name), Size: Small}).Clicked(c, "resume") {
					trs.Resume(t.ID)
				}
			} else if (IconButton{Icon: icons.Pause, Label: m.PauseTransfer(name), Size: Small}).Clicked(c, "pause") {
				trs.Pause(t.ID)
			}
			if (IconButton{Icon: icons.Close, Label: m.CancelTransfer(name), Size: Small, Danger: true}).Clicked(c, "cancel") {
				trs.Cancel(t.ID)
			}
		})
	})
}
