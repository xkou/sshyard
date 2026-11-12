// Package diagnostic writes bounded native UI diagnostics without blocking
// input or rendering on file I/O. Callers log geometry and state only.
package diagnostic

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const maxBytes int64 = 2 << 20

// Log appends JSON lines and retains one previous file as path + ".1".
type Log struct {
	path    string
	queue   chan []byte
	stop    chan struct{}
	done    chan struct{}
	mu      sync.Mutex
	closed  bool
	dropped atomic.Uint64
}

func Open(path string) (*Log, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	var size int64
	if st, err := f.Stat(); err == nil {
		size = st.Size()
	}
	l := &Log{path: path, queue: make(chan []byte, 256), stop: make(chan struct{}), done: make(chan struct{})}
	go l.write(f, size)
	return l, nil
}

func (l *Log) Event(event string, fields map[string]any) {
	if l == nil {
		return
	}
	record := struct {
		Time    string         `json:"time"`
		PID     int            `json:"pid"`
		Event   string         `json:"event"`
		Fields  map[string]any `json:"fields,omitempty"`
		Dropped uint64         `json:"dropped,omitempty"`
	}{time.Now().UTC().Format(time.RFC3339Nano), os.Getpid(), event, fields, l.dropped.Load()}
	b, err := json.Marshal(record)
	if err != nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return
	}
	select {
	case l.queue <- append(b, '\n'):
	default:
		l.dropped.Add(1)
	}
}

func (l *Log) write(f *os.File, size int64) {
	defer close(l.done)
	defer func() {
		if f != nil {
			f.Close()
		}
	}()
	for line := range l.queue {
		if size+int64(len(line)) > maxBytes {
			if f != nil {
				f.Close()
				f = nil
			}
			os.Remove(l.path + ".1")
			if err := os.Rename(l.path, l.path+".1"); err != nil && !os.IsNotExist(err) {
				continue
			}
			size = 0
		}
		if f == nil {
			var err error
			f, err = os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
			if err != nil {
				continue
			}
		}
		n, _ := f.Write(line)
		size += int64(n)
	}
}

// WatchUI distinguishes a stalled event loop from missed pointer input.
// Only one main-thread heartbeat may be pending at a time.
func (l *Log) WatchUI(post func(func()), snapshot func() map[string]any) {
	if l == nil || post == nil {
		return
	}
	var pending atomic.Int64
	var count atomic.Uint64
	nestedReported := false
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		lastStack := int64(0)
		for {
			select {
			case <-l.stop:
				return
			case now := <-ticker.C:
				if at := pending.Load(); at != 0 {
					lag := now.Sub(time.Unix(0, at)).Milliseconds()
					l.Event("ui.waiting", map[string]any{"pending_ms": lag, "goroutines": runtime.NumGoroutine()})
					if lag >= 5000 && lastStack != at {
						lastStack = at
						l.Event("ui.stack", map[string]any{"pending_ms": lag, "stack": diagnosticStack()})
					}
					continue
				}
				pending.Store(time.Now().UnixNano())
				post(func() {
					at := pending.Swap(0)
					lag := time.Since(time.Unix(0, at)).Milliseconds()
					n := count.Add(1)
					state := map[string]any{}
					if snapshot != nil {
						state = snapshot()
					}
					state["queue_ms"], state["goroutines"] = lag, runtime.NumGoroutine()
					frameActive, _ := state["frame_active"].(bool)
					frameAge, _ := state["frame_age_ms"].(int64)
					dispatchAge, _ := state["native_dispatch_ms"].(int64)
					stalled := frameActive && frameAge >= 5000 || dispatchAge >= 5000
					if stalled && !nestedReported {
						nestedReported = true
						l.Event("ui.input_stall", state)
						l.Event("ui.nested_stack", map[string]any{"stack": diagnosticStack()})
					} else if !frameActive && dispatchAge == 0 {
						nestedReported = false
					}
					if lag >= 500 || n%5 == 0 || stalled {
						l.Event("ui.heartbeat", state)
					}
				})
			}
		}
	}()
}

// diagnosticStack retains function names and source locations, stripping
// argument values from the runtime trace.
func diagnosticStack() string {
	buf := make([]byte, 128<<10)
	n := runtime.Stack(buf, true)
	lines := strings.Split(string(buf[:n]), "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, "\t") && !strings.HasPrefix(line, "created by ") && strings.HasSuffix(line, ")") {
			if at := strings.LastIndexByte(line, '('); at >= 0 {
				lines[i] = line[:at] + "(...)"
			}
		}
	}
	return strings.Join(lines, "\n")
}

func (l *Log) Close() {
	if l == nil {
		return
	}
	l.mu.Lock()
	if !l.closed {
		l.closed = true
		close(l.stop)
		close(l.queue)
	}
	l.mu.Unlock()
	select {
	case <-l.done:
	case <-time.After(time.Second):
	}
}
