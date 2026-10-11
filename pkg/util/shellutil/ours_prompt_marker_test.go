// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package shellutil

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	"github.com/wavetermdev/waveterm/pkg/blockstatus"
)

// the shell-integration part of the start-up script: from the first function after the version check to the end
func integrationBlock(t *testing.T) string {
	t.Helper()
	begin := strings.Index(PwshStartup_wavepwsh, "function Global:_waveterm_si_blocked")
	if begin < 0 {
		t.Fatalf("the shell-integration block is not in the script")
	}
	return PwshStartup_wavepwsh[begin:]
}

func promptOutput(t *testing.T, env ...string) []byte {
	t.Helper()
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("no pwsh on this machine")
	}
	script := "$Global:_WAVETERM_SI_FIRSTPROMPT = $false\n" + integrationBlock(t) + "\n[void](prompt)\n"
	cmd := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.Env = append(cmd.Environ(), env...)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("pwsh failed: %v\n%s", err, out)
	}
	return out
}

func TestThePromptFunctionPrintsTheMarkerForWavesrv(t *testing.T) {
	out := promptOutput(t, "TMUX=", "STY=", "TERM=xterm-256color")
	if !bytes.Contains(out, blockstatus.PromptMarker) {
		t.Fatalf("the prompt function must print the marker, got %q", out)
	}
	if n := bytes.Count(out, blockstatus.PromptMarker); n != 1 {
		t.Fatalf("one prompt prints one marker, got %d", n)
	}
}

func TestThePromptFunctionPrintsNoMarkerInsideTmuxOrScreen(t *testing.T) {
	out := promptOutput(t, "TMUX=/tmp/tmux-1/default,1,0")
	if bytes.Contains(out, blockstatus.PromptMarker) {
		t.Fatalf("inside tmux the integration is off, so no marker (the shell then counts as never at prompt): %q", out)
	}
}

func TestTheMarkerTheScriptPrintsIsTheOneWavesrvLooksFor(t *testing.T) {
	// the escape is spelled in the script with PowerShell's backtick-e; the bytes are ESC ] 16162 ; P BEL
	if !strings.Contains(PwshStartup_wavepwsh, "`e]16162;P`a") {
		t.Fatalf("the script must print ESC ] 16162 ; P BEL")
	}
	if string(blockstatus.PromptMarker) != "\x1b]16162;P\a" {
		t.Fatalf("the marker wavesrv looks for changed: %q", blockstatus.PromptMarker)
	}
}
