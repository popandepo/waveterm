// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// Package blockstatus turns the state of a block's program into the three words that the local API and the state record show.
// It has no dependencies, so that wsh, wavesrv and the record can all use it.
package blockstatus

import "fmt"

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

// The views that run a program under a block controller, and the views that are known to have none (the registered views of this
// build, see frontend/app/block/blockregistry.ts). A view name in neither list, such as one added by a later upstream version, is
// treated as possibly running a program until it is put in the second list on purpose.
var programViews = map[string]bool{"term": true, "tsunami": true, "vdom": true}
var noProgramViews = map[string]bool{
	"preview": true, "web": true, "waveai": true, "cpuplot": true, "sysinfo": true, "tips": true, "help": true,
	"launcher": true, "aifilediff": true, "waveconfig": true, "processviewer": true,
}

// DeleteRefusal says why a block must not be deleted by a command that has not been told to (--force), or "" when it may be. Deleting
// stops the program in the block. It is silent only when the answer is known: the view is known to have no program, or its program has
// ended ("done"). A running or starting program, a missing or unknown state, an unknown view and a view that could not be read all refuse.
// haveStatus is false when the state could not be read; shellProcStatus is "" when the block has no controller.
func DeleteRefusal(view string, shellProcStatus string, haveStatus bool) string {
	if noProgramViews[view] {
		return ""
	}
	if !programViews[view] {
		return fmt.Sprintf("the view %q is not known to be free of programs; deleting it may stop a program and cannot be undone: use --force to delete it anyway", view)
	}
	if !haveStatus {
		return "the state of the program in the block could not be read; use --force to delete it anyway"
	}
	if shellProcStatus == procDone {
		return ""
	}
	return fmt.Sprintf("a program may still be running in the block (state %q); deleting it stops the program and cannot be undone: use --force to delete it anyway", shellProcStatus)
}
