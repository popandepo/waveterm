// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package shellutil

import (
	"strings"
	"testing"
)

// A block made by `wsh run` carries the caller's whole environment. On Windows that holds names such as
// ProgramFiles(x86), CommonProgramFiles(x86) and 3DVPATH, which no shell can assign. The token exchange must still
// give the session its WAVETERM_* variables: such a name is skipped, not a reason to fail the whole exchange
// (16 Oct 2026: every session started by `wsh run` came up without wsh, swap=exit-code:1).
func TestEncodeEnvVarsSkipsNamesNoShellCanAssign(t *testing.T) {
	env := map[string]string{
		"WAVETERM_JWT":            "jwt-value",
		"WAVETERM_BLOCKID":        "block-1",
		"ProgramFiles(x86)":       `C:\Program Files (x86)`,
		"CommonProgramFiles(x86)": `C:\Program Files (x86)\Common Files`,
		"3DVPATH":                 `C:\x`,
	}
	for _, shell := range []string{ShellType_pwsh, ShellType_bash, ShellType_zsh, ShellType_fish} {
		out, err := EncodeEnvVarsForShell(shell, env)
		if err != nil {
			t.Fatalf("%s: an unassignable name failed the whole encoding: %v", shell, err)
		}
		if !strings.Contains(out, "WAVETERM_JWT") || !strings.Contains(out, "WAVETERM_BLOCKID") {
			t.Fatalf("%s: the valid variables are missing:\n%s", shell, out)
		}
		for _, bad := range []string{"ProgramFiles(x86)", "CommonProgramFiles(x86)", "3DVPATH"} {
			if strings.Contains(out, bad) {
				t.Fatalf("%s: %s should have been skipped:\n%s", shell, bad, out)
			}
		}
	}
}
