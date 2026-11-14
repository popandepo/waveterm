// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package shellexec

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// The command of a direct cmd block is the program and its arguments, with no shell before them: its Args are exactly the list given,
// quotes, backslashes and the characters ; & | stay inside one argument.
func TestDirectCommandIsTheProgramAndItsArguments(t *testing.T) {
	argv := []string{"somewhere", "-NoLogo", "-File", `C:\a b\start.ps1`, "-Prompt", `a; b & c | d "q" \x`}
	cmd := directCommand(argv)
	if len(cmd.Args) != len(argv) {
		t.Fatalf("args %q, want %q", cmd.Args, argv)
	}
	for i := range argv {
		if cmd.Args[i] != argv[i] {
			t.Fatalf("arg %d is %q, want %q", i, cmd.Args[i], argv[i])
		}
	}
}

// The real thing, on a real process: the program's parent is this process, so no shell was started in between. With the old
// "<shell> -c <line>" the parent would be the shell. Needs pwsh on the PATH (it is where the sessions run), otherwise skipped.
func TestDirectCommandHasNoShellInFront(t *testing.T) {
	if _, err := exec.LookPath("pwsh"); err != nil {
		t.Skip("pwsh is not on the PATH")
	}
	cmd := directCommand([]string{"pwsh", "-NoProfile", "-NonInteractive", "-Command", "(Get-CimInstance Win32_Process -Filter \"ProcessId=$PID\").ParentProcessId"})
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("running the command: %v", err)
	}
	parent, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatalf("parent id %q: %v", out, err)
	}
	if parent != os.Getpid() {
		t.Fatalf("the program's parent is %d, not this process %d: a shell sits in between", parent, os.Getpid())
	}
}

// The same program started the old way has a shell for a parent: the check above can tell the two apart.
func TestShellWrappedCommandHasAShellInFront(t *testing.T) {
	shell, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("pwsh is not on the PATH")
	}
	line := `pwsh -NoProfile -NonInteractive -Command '(Get-CimInstance Win32_Process -Filter "ProcessId=$PID").ParentProcessId'`
	out, err := exec.Command(shell, "-NoProfile", "-c", line).Output()
	if err != nil {
		t.Fatalf("running the wrapped command: %v", err)
	}
	parent, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	if parent == os.Getpid() || parent == 0 {
		t.Fatalf("expected a shell as the parent, got %d", parent)
	}
}
