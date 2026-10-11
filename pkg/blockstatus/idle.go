// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package blockstatus

import (
	"path/filepath"
	"strings"
)

// ChildProc is one process below a shell: its image name and the full path of its image ("" when that could not be read).
type ChildProc struct {
	Name string
	Exe  string
}

// the console host images that Windows starts next to a shell (the ConPTY host)
var consoleHelperNames = map[string]bool{
	"conhost.exe":     true,
	"openconsole.exe": true,
}

func sameDir(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// isConsoleHelper: the image is conhost.exe or OpenConsole.exe AND it lives in one of the trusted folders (the Windows system folder, Wave's own
// install folder). A program that merely carries one of these names, or whose path could not be read, is a program.
func isConsoleHelper(c ChildProc, trustedDirs []string) bool {
	if !consoleHelperNames[strings.ToLower(strings.TrimSpace(c.Name))] || c.Exe == "" {
		return false
	}
	if !consoleHelperNames[strings.ToLower(filepath.Base(c.Exe))] {
		return false
	}
	dir := filepath.Dir(c.Exe)
	for _, trusted := range trustedDirs {
		if trusted != "" && sameDir(dir, trusted) {
			return true
		}
	}
	return false
}

// ShellIsIdle says whether a local shell is waiting at its prompt: no process below it (children, their children, and so on) except console
// helpers from a trusted folder. Only a successful look can say yes: a look that failed is never idle (the question is asked).
func ShellIsIdle(below []ChildProc, lookFailed bool, trustedDirs []string) bool {
	if lookFailed {
		return false
	}
	for _, c := range below {
		if !isConsoleHelper(c, trustedDirs) {
			return false
		}
	}
	return true
}
