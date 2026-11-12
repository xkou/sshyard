# Native UI diagnostics

SSHYard writes native window and file-list diagnostics to `diagnostics.log` in its application data directory. On Linux this is normally `~/.config/SSHYard/diagnostics.log`; `SSHYARD_DATA_DIR` changes that directory. Each record is a JSON line with a UTC timestamp, process ID, event name and structured fields.

The current file is limited to 2 MiB. Its previous contents are retained as `diagnostics.log.1`. Writes run on a background worker through a bounded queue so a slow disk cannot hold up input or rendering. A `dropped` field reports records skipped when that queue was full.

Useful events:

- `window.maximize`, `window.unmaximize` and `window.resize`: native window and content bounds.
- `ui.resize` and `files.layout`: logical window size changes and recalculation of file-table layout. Earlier builds used `files.rebind` when rebuilding tables; tables now retain their identity across size changes.
- `files.geometry`: table bounds, column and row counts, visible row indexes, selection and focus state.
- `native.pointer` and `native.pointer.done` (Linux): raw GTK button presses/releases, widget class, input-device type and dispatch completion, correlated by `sequence`. Completion includes the UI runtime's pressed element, pending clicks/releases, drag, menu and modal state. Coordinates and numeric identities are recorded; widget text is not inspected.
- `files.pointer`: presses and releases delivered to a file name or the table background, with coordinates and row indexes.
- `ui.action`: a toolbar action reached its application handler, identified by the bar and action keys, without the action's label or user data.
- `files.dialog.state` and `files.dialog.paint`: a files dialog opened or closed, and its panel actually reached its paint callback. Paint records include window coordinates, pane identity and resize revision, so an active modal can be distinguished from a panel that was not painted.
- `files.marquee.begin` and `files.marquee.end`: background selection gestures and the number of selected entries.
- `ui.frame` and `ui.heartbeat`: progress of view building and the native main thread, together with input-capture state.
- `ui.waiting`: the main thread has not answered its pending heartbeat.
- `ui.stack`: one Go stack trace after a main-thread wait exceeds five seconds, with argument values removed. Function names and source locations remain for debugging.
- `ui.input_stall` and `ui.nested_stack`: a frame or native pointer dispatch has remained active for five seconds even though a nested native event loop still answers heartbeats. This separates a responsive event loop from a view that cannot finish updating.

File-list records contain geometry, numeric identities and counts. Browsed file names, directory paths, connection addresses, terminal output and credentials are not recorded.

On Linux, complete GPU frames are drawn by default. This avoids depending on GTK retaining a framebuffer across maximize and restore transitions. MyGo reads the redraw setting during package initialization, so the process performs one exec before creating windows or opening the credential store. An explicit nonempty `MYGO_PARTIAL` environment value takes precedence. `app.start` records `full_redraw` and `gpu_disabled` to identify the mode used in a reproduction.

After installing an updated build, quit SSHYard fully through its tray menu and launch it again. Reproduce the interaction and keep the approximate time. The log can then be inspected with:

```sh
tail -n 100 ~/.config/SSHYard/diagnostics.log
```

If a JSON viewer such as `jq` is available, use it to expand stack traces and other structured fields. Include both `diagnostics.log` and `diagnostics.log.1` when the interaction crossed a rotation.
