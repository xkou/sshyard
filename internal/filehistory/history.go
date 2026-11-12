// Package filehistory remembers the last successfully listed SFTP folder
// for each saved host. It writes independently of settings and layouts.
package filehistory

import (
	"encoding/json"
	"errors"
	"io/fs"
	"maps"
	"os"
	"strings"
	"sync"

	"github.com/Licoy/sshyard/internal/settings"
)

type Store struct {
	mu      sync.Mutex
	path    string
	dirs    map[string]string
	wake    chan struct{}
	stop    chan struct{}
	done    chan struct{}
	closed  bool
	onError func(error)
}

// Open loads metadata without accessing saved credentials. A read error
// returns a usable memory store and leaves the unreadable file untouched.
func Open(path string, onError func(error)) (*Store, error) {
	s := &Store{dirs: map[string]string{}, onError: onError}
	if path == "" {
		return s, nil
	}
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return s, err
	}
	if err == nil {
		if err := json.Unmarshal(b, &s.dirs); err != nil {
			s.dirs = map[string]string{}
			return s, err
		}
	}
	if s.dirs == nil {
		s.dirs = map[string]string{}
	}
	for host, dir := range s.dirs {
		if !valid(host, dir) {
			delete(s.dirs, host)
		}
	}
	s.path, s.wake, s.stop, s.done = path, make(chan struct{}, 1), make(chan struct{}), make(chan struct{})
	go s.run()
	return s, nil
}

func valid(host, dir string) bool {
	return host != "" && dir != "" && !strings.ContainsRune(host, 0) && !strings.ContainsRune(dir, 0)
}

func (s *Store) Get(host string) string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dirs[host]
}

// Remember updates memory immediately and coalesces disk writes on a worker.
func (s *Store) Remember(host, dir string) {
	if s == nil || !valid(host, dir) {
		return
	}
	s.mu.Lock()
	if s.closed || s.dirs[host] == dir {
		s.mu.Unlock()
		return
	}
	s.dirs[host] = dir
	wake := s.wake
	s.mu.Unlock()
	if wake != nil {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
}

func (s *Store) save() error {
	s.mu.Lock()
	dirs := maps.Clone(s.dirs)
	s.mu.Unlock()
	b, err := json.MarshalIndent(dirs, "", "  ")
	if err != nil {
		return err
	}
	return settings.WriteFileAtomic(s.path, b, 0o600)
}

func (s *Store) run() {
	defer close(s.done)
	failed := false
	flush := func() {
		err := s.save()
		if err != nil && !failed && s.onError != nil {
			s.onError(err)
		}
		failed = err != nil
	}
	for {
		select {
		case <-s.wake:
			flush()
		case <-s.stop:
			flush()
			return
		}
	}
}

// Close flushes the latest folder before the app releases its data lock.
func (s *Store) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		if s.stop != nil {
			close(s.stop)
		}
	}
	done := s.done
	s.mu.Unlock()
	if done != nil {
		<-done
	}
}
