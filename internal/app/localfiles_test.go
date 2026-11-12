package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Licoy/sshyard/internal/keychain"
	"github.com/Licoy/sshyard/internal/layout"
	"github.com/Licoy/sshyard/internal/sftpsvc"
	"github.com/Licoy/sshyard/locales"
)

func TestLocalFilesWithoutSecretsOrShell(t *testing.T) {
	withShells(t)
	backend := &authorizationBackend{Memory: keychain.NewMemory()}
	a, tt := lockedTestApp(t, backend)
	write(t, filepath.Join(a.p.HomeDir, "local.txt"), "local contents\n")
	m := locales.In("en")
	opened := false
	for _, entry := range a.addEntries(m) {
		if entry.Label == m.LocalFiles() {
			entry.Do()
			opened = true
			break
		}
	}
	if !opened {
		t.Fatal("no local files action")
	}
	tt.Frame()
	cn := a.focused()
	if cn == nil || cn.kind != layout.Files || cn.host.ID != localID || cn.s != nil {
		t.Fatalf("local browser started a session or did not open: %+v", cn)
	}
	if !cn.fs.local.loaded || cn.fs.local.dir != a.p.HomeDir || !slices.ContainsFunc(cn.fs.local.entries, func(e sftpsvc.Entry) bool { return e.Name == "local.txt" }) {
		t.Fatal("local browser did not list this computer's home folder")
	}
	if cn.fs.remote.client != nil || cn.fs.remote.opening {
		t.Fatal("local browser tried to open SFTP")
	}
	if _, err := cn.fs.transfers.Client(context.Background()); !errors.Is(err, errNotConnected) {
		t.Fatalf("remote transfer from a local browser: %v", err)
	}
	if backend.calls.Load() != 0 || a.unlocked() || a.unlock.modal {
		t.Fatal("local browser requested saved secrets")
	}
	id, err := a.OpenLocalFiles()
	if err != nil || id == cn.id || len(a.ws.Tabs) != 2 {
		t.Fatalf("local files did not open in a new tab: id=%d err=%v", id, err)
	}
	a.CloseSession(id)
	if a.conns[cn.id] != cn {
		t.Fatal("closing another local browser closed the first")
	}
}

func TestLocalFilesOperations(t *testing.T) {
	a, _ := newTestApp(t)
	id, err := a.OpenLocalFiles()
	if err != nil {
		t.Fatal(err)
	}
	cn := a.conns[id]
	a.listLocalFirst(cn)
	if err := a.Mkdir(cn, false, "work"); err != nil {
		t.Fatal(err)
	}
	for _, entry := range cn.fs.local.entries {
		if entry.Name == "work" {
			a.openEntry(cn, false, entry)
			break
		}
	}
	dir := filepath.Join(a.p.HomeDir, "work")
	if cn.fs.local.dir != dir {
		t.Fatalf("opened %q, want %q", cn.fs.local.dir, dir)
	}
	if err := a.doAsk(cn, &nameAsk{kind: askNewFile, value: "new.txt"}); err != nil {
		t.Fatal(err)
	}
	old, renamed := filepath.Join(dir, "new.txt"), filepath.Join(dir, "renamed.txt")
	if err := a.doAsk(cn, &nameAsk{kind: askRename, path: old, value: "renamed.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(renamed); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old file survived rename: %v", err)
	}
	if !slices.ContainsFunc(cn.fs.local.entries, func(e sftpsvc.Entry) bool { return e.Name == "renamed.txt" }) || cn.s != nil {
		t.Fatal("local operations did not refresh the browser or started a session")
	}
}

func TestLocalFilesLayoutRestoreWithoutShell(t *testing.T) {
	dir, backend := t.TempDir(), keychain.NewMemory()
	rememberLayoutIn(t, dir)
	a, _ := appIn(t, dir, backend)
	id, err := a.OpenLocalFiles()
	if err != nil {
		t.Fatal(err)
	}
	a.conns[id].fs.local.dir = filepath.Join(dir, "PRIVATE_LOCAL_DIRECTORY")
	a.Close()
	raw, err := os.ReadFile(filepath.Join(dir, "layout.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "PRIVATE_LOCAL_DIRECTORY") {
		t.Fatal("layout saved the local browser's directory")
	}
	withShells(t)
	again, tt := appIn(t, dir, backend)
	tt.Frame()
	cn := again.focused()
	if cn == nil || cn.kind != layout.Files || cn.host.ID != localID || cn.s != nil {
		t.Fatalf("local browser did not restore without a shell: %+v", cn)
	}
	if !cn.fs.local.loaded || cn.fs.local.dir != dir {
		t.Fatal("restored local browser did not start at home")
	}
}
