// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// Package blockstatus turns the state of a block's program into the three words that the local API and the state record show.
// It has no dependencies, so that wsh, wavesrv and the record can all use it.
package blockstatus

const (
	Running    = "running"
	Exited     = "exited"
	NotStarted = "notstarted"

	// the values the block controller uses for ShellProcStatus
	procRunning = "running"
	procDone    = "done"
	procInit    = "init"
)

// FromShellProc maps a block controller's ShellProcStatus (and exit code) to a status word and, for an exited program, its
// exit code. A block without a controller, or a status this code does not know, gives no status: unknown is not "running".
func FromShellProc(shellProcStatus string, exitCode int) (string, *int) {
	switch shellProcStatus {
	case procRunning:
		return Running, nil
	case procDone:
		code := exitCode
		return Exited, &code
	case procInit:
		return NotStarted, nil
	}
	return "", nil
}
