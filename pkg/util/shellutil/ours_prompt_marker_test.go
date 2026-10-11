// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package shellutil

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/wavetermdev/waveterm/pkg/blockstatus"
)

const testPromptToken = "testtoken0123456789abcdef"

// the part of the start-up script that reads the token, and the shell-integration block
func tokenAndIntegration(t *testing.T) string {
	t.Helper()
	tokenBegin := strings.Index(PwshStartup_wavepwsh, "$waveToken = $env:WAVETERM_PROMPT_TOKEN")
	tokenEnd := strings.Index(PwshStartup_wavepwsh, "# Load Wave completions")
	begin := strings.Index(PwshStartup_wavepwsh, "function Global:_waveterm_si_blocked")
	if tokenBegin < 0 || tokenEnd < tokenBegin || begin < 0 {
		t.Fatalf("the token or the shell-integration block is not in the script")
	}
	return PwshStartup_wavepwsh[tokenBegin:tokenEnd] + "$Global:_WAVETERM_SI_FIRSTPROMPT = $false\n" + PwshStartup_wavepwsh[begin:]
}

// a real PowerShell 7 under a real ConPTY (the pty library Wave uses), headless, feeding a PromptTracker exactly as the controller does
type conptyShell struct {
	t       *testing.T
	cmd     *exec.Cmd
	pty     pty.Pty
	tracker *blockstatus.PromptTracker
	lock    sync.Mutex
	out     bytes.Buffer
}

func startShell(t *testing.T, beforeIntegration string, env ...string) *conptyShell {
	t.Helper()
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("no pwsh on this machine")
	}
	script := beforeIntegration + "\n" + tokenAndIntegration(t)
	// as Wave starts it: -NoExit -File <script>, so that the script has a scope of its own (not the global one)
	scriptPath := filepath.Join(t.TempDir(), "wavepwsh-test.ps1")
	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(pwsh, "-NoLogo", "-NoProfile", "-ExecutionPolicy", "Bypass", "-NoExit", "-File", scriptPath)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "TMUX=", "STY=", "WAVETERM_PROMPT_TOKEN="+testPromptToken)
	cmd.Env = append(cmd.Env, env...)
	p, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 30, Cols: 100})
	if err != nil {
		t.Fatal(err)
	}
	s := &conptyShell{t: t, cmd: cmd, pty: p, tracker: &blockstatus.PromptTracker{}}
	s.tracker.SetToken(testPromptToken)
	go func() {
		buf := make([]byte, 8192)
		for {
			n, err := p.Read(buf)
			if n > 0 {
				s.tracker.Output(buf[:n])
				s.lock.Lock()
				s.out.Write(buf[:n])
				s.lock.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		p.Write([]byte("exit\r"))
		time.Sleep(200 * time.Millisecond)
		cmd.Process.Kill()
	})
	return s
}

func (s *conptyShell) send(text string) {
	s.tracker.Input([]byte(text)) // as the controller does, before the write
	if _, err := s.pty.Write([]byte(text)); err != nil {
		s.t.Fatal(err)
	}
}

func (s *conptyShell) output() string {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.out.String()
}

func (s *conptyShell) atPrompt() bool {
	v, _ := s.tracker.AtPrompt()
	return v
}

func (s *conptyShell) waitFor(what string, limit time.Duration, cond func() bool) {
	s.t.Helper()
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	s.t.Fatalf("timed out waiting for: %s\noutput so far: %q", what, s.output())
}

func TestARealShellIsAtPromptWhenItStartsAndNotWhileACommandRuns(t *testing.T) {
	s := startShell(t, "")
	s.waitFor("the first prompt", 15*time.Second, s.atPrompt)
	s.send("Start-Sleep 2\r")
	time.Sleep(500 * time.Millisecond)
	if s.atPrompt() {
		t.Fatalf("a command that runs inside the shell (no process below it) must not count as at prompt")
	}
	s.waitFor("the prompt after the command", 10*time.Second, s.atPrompt)
}

func TestARealShellIsAtPromptRightAfterAQuickCommandWithoutWaitingForTheResync(t *testing.T) {
	// the Enter binding reports B, so the Enter is not left pending: the next prompt marker counts at once. Without B this would need the whole
	// resync wait (a second or more), so the limit here is well under it.
	s := startShell(t, "")
	s.waitFor("the first prompt", 15*time.Second, s.atPrompt)
	for i := 0; i < 3; i++ {
		s.send("Get-Date | Out-Null\r")
		time.Sleep(150 * time.Millisecond)
		s.waitFor("the prompt right after a quick command", 1500*time.Millisecond, s.atPrompt)
	}
}

func TestARealShellWithATypedAheadCommandIsNotAtPromptUntilBothAreDone(t *testing.T) {
	s := startShell(t, "")
	s.waitFor("the first prompt", 15*time.Second, s.atPrompt)
	s.send("Start-Sleep 2\r")
	time.Sleep(300 * time.Millisecond)
	s.send("Start-Sleep 2\r") // typed ahead while the first runs
	// the first command's prompt marker arrives after about 2 s, with the second Enter still queued: at no moment until both are done may it say at prompt
	deadline := time.Now().Add(3200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if s.atPrompt() {
			t.Fatalf("at prompt while a typed-ahead command is queued or running")
		}
		time.Sleep(25 * time.Millisecond)
	}
	s.waitFor("the prompt after both", 10*time.Second, s.atPrompt)
}

func TestARealShellWhoseEnterBindingWasReplacedAfterwardsReportsItAndIsNeverAtPrompt(t *testing.T) {
	s := startShell(t, "")
	s.waitFor("the first prompt", 15*time.Second, s.atPrompt)
	s.send("Set-PSReadLineKeyHandler -Key Enter -ScriptBlock { [Microsoft.PowerShell.PSConsoleReadLine]::AcceptLine() }\r")
	time.Sleep(1500 * time.Millisecond) // the Enter that ran this command was still ours (B came), the next prompt reports N
	s.send("Get-Date\r")
	time.Sleep(2500 * time.Millisecond)
	if s.atPrompt() {
		t.Fatalf("a shell whose Enter binding is no longer ours must never count as at prompt\noutput: %q", s.output())
	}
	if !strings.Contains(s.output(), "16162;N;"+testPromptToken) {
		t.Fatalf("the next prompt must report N, output: %q", s.output())
	}
}

func TestARealShellWhoseProfileSetItsOwnEnterBindingIsLeftAloneAndNeverAtPrompt(t *testing.T) {
	s := startShell(t, "Set-PSReadLineKeyHandler -Key Enter -ScriptBlock { [Microsoft.PowerShell.PSConsoleReadLine]::AcceptLine() }")
	time.Sleep(3 * time.Second)
	if s.atPrompt() {
		t.Fatalf("a custom Enter binding must not be taken over, and no marker means never at prompt")
	}
	if strings.Contains(s.output(), "16162;P;") || strings.Contains(s.output(), "16162;B;") {
		t.Fatalf("no marker may be printed when the Enter binding is not ours: %q", s.output())
	}
	s.send("Get-Date\r")
	time.Sleep(1500 * time.Millisecond)
	if !strings.Contains(s.output(), "20") { // the shell still works: Get-Date printed a year
		t.Fatalf("the shell must still work: %q", s.output())
	}
}

func TestARealShellThatAnsweredAProgramIsAtPromptAgainAfterTheResyncWait(t *testing.T) {
	s := startShell(t, "")
	s.waitFor("the first prompt", 15*time.Second, s.atPrompt)
	s.send("Read-Host 'name'\r")
	time.Sleep(700 * time.Millisecond)
	s.send("answer\r") // consumed by Read-Host: it goes through the same Enter key binding (B), unlike a program that reads the console itself
	s.waitFor("the prompt after Read-Host (with or without the resync wait)", 10*time.Second, s.atPrompt)
}

func TestTheTokenIsRemovedFromTheEnvironmentOfTheShellAndOfProgramsStartedFromIt(t *testing.T) {
	dir := t.TempDir()
	own := filepath.Join(dir, "own.txt")
	child := filepath.Join(dir, "child.txt")
	s := startShell(t, "")
	s.waitFor("the first prompt", 15*time.Second, s.atPrompt)
	s.send("[string][Environment]::GetEnvironmentVariable('WAVETERM_PROMPT_TOKEN') | Set-Content -LiteralPath '" + own + "'\r")
	s.waitFor("the shell's own answer", 10*time.Second, func() bool { _, err := os.Stat(own); return err == nil })
	s.waitFor("the prompt", 10*time.Second, s.atPrompt)
	s.send("pwsh -NoProfile -Command \"[string]$env:WAVETERM_PROMPT_TOKEN | Set-Content -LiteralPath '" + child + "'\"\r")
	s.waitFor("the child's answer", 20*time.Second, func() bool { _, err := os.Stat(child); return err == nil })
	time.Sleep(300 * time.Millisecond)
	for name, path := range map[string]string{"the shell itself": own, "a program started from it": child} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), testPromptToken) {
			t.Errorf("%s must not see the token, got %q", name, data)
		}
	}
}

func TestTheMarkerNamesInTheScriptAreTheOnesWavesrvLooksFor(t *testing.T) {
	for _, want := range []string{"`e]16162;B;$waveToken`a", "`e]16162;P;$waveToken`a", "`e]16162;Q;$waveToken`a", "`e]16162;N;$waveToken`a"} {
		if !strings.Contains(PwshStartup_wavepwsh, want) {
			t.Errorf("the script must print %s", want)
		}
	}
	if !strings.Contains(PwshStartup_wavepwsh, "Remove-Item Env:WAVETERM_PROMPT_TOKEN") {
		t.Errorf("the script must remove the token from the environment")
	}
	if strings.Contains(PwshStartup_wavepwsh, "$Global:_WAVETERM_PROMPT_TOKEN") {
		t.Errorf("no global variable may hold the token")
	}
	// the token must reach the script after the profile has run: through the swap token (applied by the script itself), not the launch environment
	swap := strings.Index(PwshStartup_wavepwsh, "Invoke-Expression $waveterm_swaptoken_output")
	read := strings.Index(PwshStartup_wavepwsh, "$waveToken = $env:WAVETERM_PROMPT_TOKEN")
	if swap < 0 || read < swap {
		t.Errorf("the token is read after the swap token has been applied")
	}
}

func TestTheTokenIsNotInAnyGlobalVariableOfTheShell(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "globals.txt")
	s := startShell(t, "")
	s.waitFor("the first prompt", 15*time.Second, s.atPrompt)
	s.send("(Get-Variable -Scope Global | Where-Object { \"$($_.Value)\" -like '*" + testPromptToken + "*' } | Measure-Object).Count | Set-Content -LiteralPath '" + out + "'\r")
	s.waitFor("the answer", 10*time.Second, func() bool { _, err := os.Stat(out); return err == nil })
	time.Sleep(300 * time.Millisecond)
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != "0" {
		t.Fatalf("a global variable holds the token (count %q)", data)
	}
}

func TestAKeyBoundToAnAcceptingFunctionOtherThanEnterReportsNotOursAtTheNextPrompt(t *testing.T) {
	s := startShell(t, "")
	s.waitFor("the first prompt", 15*time.Second, s.atPrompt)
	s.send("Set-PSReadLineKeyHandler -Key Tab -Function AcceptLine\r")
	time.Sleep(1500 * time.Millisecond)
	if s.atPrompt() {
		t.Fatalf("with Tab bound to AcceptLine the shell must not count as at prompt\noutput: %q", s.output())
	}
	if !strings.Contains(s.output(), "16162;N;"+testPromptToken) {
		t.Fatalf("the next prompt must report N, output: %q", s.output())
	}
}

func TestTheShellSaysNothingIsQueuedOnlyWhenNothingIsQueued(t *testing.T) {
	// "Start-Sleep 2" and, typed ahead, a second command: the first command's prompt must NOT carry Q (a key is waiting), the last one must
	s := startShell(t, "")
	s.waitFor("the first prompt", 15*time.Second, s.atPrompt)
	s.waitFor("Q at the start (nothing queued)", 5*time.Second, func() bool { return strings.Contains(s.output(), "16162;Q;"+testPromptToken) })
	before := strings.Count(s.output(), "16162;Q;"+testPromptToken)
	pBefore := strings.Count(s.output(), "16162;P;"+testPromptToken)
	bBefore := strings.Count(s.output(), "16162;B;"+testPromptToken)
	s.send("Start-Sleep 2\r")
	time.Sleep(300 * time.Millisecond)
	s.send("Start-Sleep 1\r") // typed ahead
	s.waitFor("both done", 15*time.Second, func() bool {
		return s.atPrompt() && strings.Count(s.output(), "16162;P;"+testPromptToken) >= pBefore+2
	})
	if bs := strings.Count(s.output(), "16162;B;"+testPromptToken) - bBefore; bs != 2 {
		t.Fatalf("the shell must report each Enter it consumed (marker B): want 2, got %d\noutput: %q", bs, s.output())
	}
	ps := strings.Count(s.output(), "16162;P;"+testPromptToken) - pBefore
	qs := strings.Count(s.output(), "16162;Q;"+testPromptToken) - before
	if ps != 2 || qs != 1 {
		t.Fatalf("two prompts, only the last without a queued key: want P=2 Q=1, got P=%d Q=%d\noutput: %q", ps, qs, s.output())
	}
}
