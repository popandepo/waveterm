// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package blockstatus

import "strings"

// the console host processes that Windows starts next to a shell: they are not a program the user is running
var consoleHelpers = map[string]bool{
	"conhost.exe":     true,
	"openconsole.exe": true,
	"conhost":         true,
	"openconsole":     true,
}

// ShellIsIdle says whether a local shell is waiting at its prompt: it has no child process other than a console helper. Only a successful look
// can say yes: a look that failed is never idle (the question is asked).
func ShellIsIdle(childNames []string, lookFailed bool) bool {
	if lookFailed {
		return false
	}
	for _, name := range childNames {
		if !consoleHelpers[strings.ToLower(strings.TrimSpace(name))] {
			return false
		}
	}
	return true
}
