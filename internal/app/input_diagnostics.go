package app

import (
	"reflect"
	"time"

	"github.com/egoist/mygo/ui"
)

// DiagnosticInputSnapshot reads only numeric input state from the pinned
// MyGo runtime. It runs on the UI thread, including from a nested GTK loop.
// No text, paths or credentials are inspected, and framework state stays
// unchanged. Missing fields are omitted if the SDK's representation changes.
func (a *App) DiagnosticInputSnapshot() map[string]any {
	d := &a.uiTrace
	fields := map[string]any{"frame": d.frame, "revision": d.revision, "view_active": d.viewActive, "view_ms": int64(0), "frame_age_ms": int64(0), "page": int(a.page), "ready": a.ready()}
	if d.viewActive {
		fields["view_ms"] = time.Since(d.viewStarted).Milliseconds()
	}
	if !d.frameAt.IsZero() {
		fields["frame_age_ms"] = time.Since(d.frameAt).Milliseconds()
	}
	if a.mu.TryLock() {
		fields["app_mutex_busy"] = false
		fields["queued_callbacks"] = len(a.queue)
		fields["callback_posted"] = a.posted
		a.mu.Unlock()
	} else {
		fields["app_mutex_busy"] = true
	}
	rt := diagnosticField(reflect.ValueOf(d.context), "rt")
	if !rt.IsValid() || rt.Kind() != reflect.Pointer || rt.IsNil() {
		fields["runtime_available"] = false
		return fields
	}
	fields["runtime_available"] = true
	for key, path := range map[string][]string{
		"frame_active": {"inFrame"}, "runtime_closed": {"closed"},
		"runtime_frame": {"frame"}, "paint_count": {"paints"},
		"window_focused": {"windowFocused"}, "pointer_in": {"pointerIn"},
		"pointer_x": {"pointerX"}, "pointer_y": {"pointerY"},
		"press_button": {"pressButton"}, "press_depth": {"downing"},
		"modal_id": {"modal"}, "modal_layer": {"modalLayer"},
		"focused_id": {"focused"}, "last_press_id": {"lastPress", "id"},
		"pressed_id": {"pressed", "id"}, "pressed_flags": {"pressed", "flags"},
		"pressed_scope": {"pressed", "scope"},
		"drag_source":   {"drag", "src"}, "native_drag": {"drag", "native"},
		"drag_cancelled": {"drag", "canceled"}, "menu_release": {"menu", "release"},
	} {
		if value, ok := diagnosticScalar(diagnosticField(rt, path...)); ok {
			fields[key] = value
		}
	}
	if active, _ := fields["frame_active"].(bool); active && d.context != nil {
		if at := d.context.Now(); !at.IsZero() {
			fields["frame_age_ms"] = time.Since(at).Milliseconds()
		}
	}
	for key, path := range map[string][]string{
		"pressed": {"pressed"}, "drag_active": {"drag"},
		"scroll_drag": {"scrollDrag", "st"}, "native_menu_pending": {"menu", "pending"},
		"drawn_menu": {"menu", "drawn"},
	} {
		v := diagnosticField(rt, path...)
		fields[key] = v.IsValid() && v.Kind() == reflect.Pointer && !v.IsNil()
	}
	if v := diagnosticField(rt, "ups"); v.IsValid() && v.Kind() == reflect.Slice {
		fields["queued_releases"] = v.Len()
	}
	if hover := diagnosticField(rt, "hover"); hover.IsValid() && hover.Kind() == reflect.Slice && hover.Len() > 0 {
		if value, ok := diagnosticScalar(hover.Index(0)); ok {
			fields["hover_id"] = value
		}
	}
	id := diagnosticField(rt, "lastPress", "id")
	states := diagnosticField(rt, "states")
	if id.IsValid() && states.IsValid() && states.Kind() == reflect.Map && id.Type() == states.Type().Key() {
		st := states.MapIndex(id)
		for key, field := range map[string]string{"target_clicks": "clicks", "target_flags": "flags", "target_seen": "seen", "target_pressed": "pressed", "target_ignored": "ignoreInput"} {
			if value, ok := diagnosticScalar(diagnosticField(st, field)); ok {
				fields[key] = value
			}
		}
	}
	return fields
}

func diagnosticField(v reflect.Value, path ...string) reflect.Value {
	for _, field := range path {
		for v.IsValid() && (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) {
			if v.IsNil() {
				return reflect.Value{}
			}
			v = v.Elem()
		}
		if !v.IsValid() || v.Kind() != reflect.Struct {
			return reflect.Value{}
		}
		v = v.FieldByName(field)
	}
	return v
}

func diagnosticScalar(v reflect.Value) (any, bool) {
	if !v.IsValid() {
		return nil, false
	}
	switch v.Kind() {
	case reflect.Bool:
		return v.Bool(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int(), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint(), true
	case reflect.Float32, reflect.Float64:
		return v.Float(), true
	}
	return nil, false
}

// beginUIView tracks view construction separately from event-loop liveness:
// a synchronous GTK operation can keep dispatching events inside a frame.
func (a *App) beginUIView(c *ui.Context) func() {
	d := &a.uiTrace
	d.context, d.viewActive, d.viewStarted = c, true, time.Now()
	return func() { d.viewActive = false }
}
