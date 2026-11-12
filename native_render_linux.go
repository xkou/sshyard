//go:build linux

package main

import (
	"os"
	"syscall"
)

// prepareNativeRendering makes GTK's GL renderer redraw complete frames.
// GtkGLArea can replace its framebuffer during a maximize transition;
// retaining partial frames then leaves selection and hover changes invisible.
// MyGo reads MYGO_PARTIAL during package initialization, so changing the
// default requires one exec before opening windows or the credential store.
func prepareNativeRendering() error {
	if value, set := os.LookupEnv("MYGO_PARTIAL"); set && value != "" {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := os.Setenv("MYGO_PARTIAL", "0"); err != nil {
		return err
	}
	return syscall.Exec(exe, os.Args, os.Environ())
}
