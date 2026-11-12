//go:build linux

package gtkfix

import (
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

var pointerTrace struct {
	callback                     uintptr
	report                       func(string, map[string]any)
	sequence                     uint64
	buttons                      uint32
	lastDown, lastUp, lastMotion time.Time
	depth                        int
	started                      time.Time
	getGrab                      func() uintptr
	typeName                     func(uintptr) uintptr
}

// TraceInput records pointer dispatch before GTK's widget handlers and after
// they return. It forwards every event through GTK's standard dispatcher.
// Install once on the main thread after GTK has initialized.
func TraceInput(report func(string, map[string]any)) bool {
	if report == nil {
		return false
	}
	if pointerTrace.callback != 0 {
		pointerTrace.report = report
		return true
	}
	var setHandler func(uintptr, uintptr, uintptr)
	var dispatch func(uintptr)
	var eventType func(uintptr) int32
	var getButton func(uintptr, *uint32) bool
	var getState func(uintptr, *uint32) bool
	var getCoords func(uintptr, *float64, *float64) bool
	var getWidget, getSource func(uintptr) uintptr
	var sourceType func(uintptr) int32
	bind := func(fn any, name string) bool {
		p := sym(name)
		if p == 0 {
			return false
		}
		purego.RegisterFunc(fn, p)
		return true
	}
	if !bind(&setHandler, "gdk_event_handler_set") || !bind(&dispatch, "gtk_main_do_event") ||
		!bind(&eventType, "gdk_event_get_event_type") || !bind(&getButton, "gdk_event_get_button") ||
		!bind(&getState, "gdk_event_get_state") || !bind(&getCoords, "gdk_event_get_coords") ||
		!bind(&getWidget, "gtk_get_event_widget") || !bind(&getSource, "gdk_event_get_source_device") ||
		!bind(&sourceType, "gdk_device_get_source") || !bind(&pointerTrace.getGrab, "gtk_grab_get_current") ||
		!bind(&pointerTrace.typeName, "g_type_name_from_instance") {
		return false
	}
	pointerTrace.report = report
	pointerTrace.callback = purego.NewCallback(func(event, data uintptr) {
		typ := eventType(event)
		if typ == 3 { // GDK_MOTION_NOTIFY
			var state uint32
			getState(event, &state)
			pointerTrace.buttons = state & 0x1f00
			pointerTrace.lastMotion = time.Now()
		}
		if typ != 4 && typ != 7 { // GDK_BUTTON_PRESS / GDK_BUTTON_RELEASE
			dispatch(event)
			return
		}
		var button, state uint32
		var x, y float64
		getButton(event, &button)
		getState(event, &state)
		getCoords(event, &x, &y)
		pointerTrace.buttons = state & 0x1f00
		if button >= 1 && button <= 5 {
			mask := uint32(1) << (7 + button)
			if typ == 4 {
				pointerTrace.buttons |= mask
			} else {
				pointerTrace.buttons &^= mask
			}
		}
		if typ == 4 {
			pointerTrace.lastDown = time.Now()
		} else {
			pointerTrace.lastUp = time.Now()
		}
		pointerTrace.sequence++
		seq := pointerTrace.sequence
		source := int32(-1)
		if device := getSource(event); device != 0 {
			source = sourceType(device)
		}
		fields := map[string]any{"sequence": seq, "kind": typ, "button": button, "state": state, "buttons": pointerTrace.buttons, "x": x, "y": y, "widget_type": pointerWidgetType(getWidget(event)), "source_type": source}
		pointerTrace.report("native.pointer", fields)
		depth, started := pointerTrace.depth, pointerTrace.started
		pointerTrace.depth, pointerTrace.started = depth+1, time.Now()
		dispatch(event)
		elapsed := time.Since(pointerTrace.started).Milliseconds()
		pointerTrace.depth, pointerTrace.started = depth, started
		pointerTrace.report("native.pointer.done", map[string]any{"sequence": seq, "kind": typ, "button": button, "dispatch_ms": elapsed})
	})
	setHandler(pointerTrace.callback, 0, 0)
	return true
}

func pointerWidgetType(widget uintptr) string {
	if widget == 0 || pointerTrace.typeName == nil {
		return ""
	}
	p := pointerTrace.typeName(widget)
	if p == 0 {
		return ""
	}
	// GTK class names are static type metadata, not widget labels.
	n := 0
	for n < 128 && *(*byte)(unsafe.Pointer(p + uintptr(n))) != 0 {
		n++
	}
	return string(unsafe.Slice((*byte)(unsafe.Pointer(p)), n))
}

// InputState reads cached native pointer state on the GTK main thread.
func InputState() map[string]any {
	if pointerTrace.callback == 0 {
		return nil
	}
	age := func(at time.Time) int64 {
		if at.IsZero() {
			return -1
		}
		return time.Since(at).Milliseconds()
	}
	fields := map[string]any{
		"native_sequence":       pointerTrace.sequence,
		"native_buttons":        pointerTrace.buttons,
		"native_down_age_ms":    age(pointerTrace.lastDown),
		"native_up_age_ms":      age(pointerTrace.lastUp),
		"native_motion_age_ms":  age(pointerTrace.lastMotion),
		"native_dispatch_depth": pointerTrace.depth,
		"native_dispatch_ms":    int64(0),
		"gtk_grab_type":         pointerWidgetType(pointerTrace.getGrab()),
	}
	if pointerTrace.depth > 0 {
		fields["native_dispatch_ms"] = age(pointerTrace.started)
	}
	return fields
}
