// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package shellutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wavetermdev/waveterm/pkg/wavebase"
)

const testBlockId = "0a1b2c3d-1111-2222-3333-444455556666"

func boolPtr(b bool) *bool { return &b }

func useDataDir(t *testing.T) string {
	t.Helper()
	old := wavebase.DataHome_VarCache
	dir := t.TempDir()
	wavebase.DataHome_VarCache = dir
	t.Cleanup(func() { wavebase.DataHome_VarCache = old })
	return dir
}

func TestOnlyAUuidBlockIdGivesAPathInsideTheHistoryFolder(t *testing.T) {
	dir := useDataDir(t)
	want := filepath.Join(dir, "history", testBlockId+".txt")
	if got := BlockHistoryFile(dir, testBlockId); got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
	for _, bad := range []string{"", "..", `..\x`, "../x", testBlockId + "/x", testBlockId + ".txt", strings.ToUpper(testBlockId), "short"} {
		if got := BlockHistoryFile(dir, bad); got != "" {
			t.Errorf("%q must give no path, got %q", bad, got)
		}
	}
}

func TestPerBlockHistoryIsOnUnlessSwitchedOffAndOnlyForLocalPowerShell(t *testing.T) {
	dir := useDataDir(t)
	want := filepath.Join(dir, "history", testBlockId+".txt")
	for _, enabled := range []*bool{nil, boolPtr(true)} {
		for _, conn := range []string{"", "local", "local:gitbash"} {
			env := PerBlockHistoryEnv(enabled, ShellType_pwsh, conn, testBlockId)
			if env[PwshHistFileEnvName] != want {
				t.Errorf("enabled=%v conn=%q: want %q, got %v", enabled, conn, want, env)
			}
		}
	}
	if env := PerBlockHistoryEnv(boolPtr(false), ShellType_pwsh, "", testBlockId); env != nil {
		t.Errorf("switched off must give nothing: %v", env)
	}
	if env := PerBlockHistoryEnv(nil, "bash", "", testBlockId); env != nil {
		t.Errorf("bash is left alone: %v", env)
	}
	if env := PerBlockHistoryEnv(nil, ShellType_pwsh, "user@server", testBlockId); env != nil {
		t.Errorf("a remote connection has no history file on this machine: %v", env)
	}
	if env := PerBlockHistoryEnv(nil, ShellType_pwsh, "", "not-a-uuid"); env != nil {
		t.Errorf("a bad block id gives nothing: %v", env)
	}
	if st, err := os.Stat(filepath.Join(dir, "history")); err != nil || !st.IsDir() {
		t.Errorf("the history folder is made when a file is handed out: %v", err)
	}
}

func TestRemovingTheHistoryOfABlockRemovesThatOneFile(t *testing.T) {
	dir := useDataDir(t)
	other := "ffffffff-1111-2222-3333-444455556666"
	if err := os.MkdirAll(filepath.Join(dir, "history"), 0700); err != nil {
		t.Fatal(err)
	}
	mine := BlockHistoryFile(dir, testBlockId)
	theirs := BlockHistoryFile(dir, other)
	for _, f := range []string{mine, theirs} {
		if err := os.WriteFile(f, []byte("echo hi\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := RemoveBlockHistory(dir, testBlockId); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(mine); !os.IsNotExist(err) {
		t.Fatalf("the file of the deleted block must be gone: %v", err)
	}
	if _, err := os.Stat(theirs); err != nil {
		t.Fatalf("another block's file must stay: %v", err)
	}
	if err := RemoveBlockHistory(dir, testBlockId); err != nil {
		t.Fatalf("a missing file is not an error: %v", err)
	}
	if err := RemoveBlockHistory(dir, "../../etc/passwd"); err != nil {
		t.Fatalf("a bad id is ignored: %v", err)
	}
}

// The history block of the start-up script, run in a real PowerShell (skipped where there is none). It must set PSReadLine's save path
// from the environment, remove the variable, and never break the shell start when the line editor is missing or fails.
func historyBlock(t *testing.T) string {
	t.Helper()
	begin := strings.Index(PwshStartup_wavepwsh, "# wave-history-begin")
	end := strings.Index(PwshStartup_wavepwsh, "# wave-history-end")
	if begin < 0 || end < begin {
		t.Fatalf("the history block is not marked in the start-up script")
	}
	return PwshStartup_wavepwsh[begin:end]
}

func runPwsh(t *testing.T, script string, env ...string) string {
	t.Helper()
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("no pwsh on this machine")
	}
	cmd := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pwsh failed: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestThePwshStartupSetsTheHistoryPathAndRemovesTheVariable(t *testing.T) {
	file := filepath.Join(t.TempDir(), "block-history.txt")
	script := historyBlock(t) + "\nWrite-Output ('path=' + (Get-PSReadLineOption).HistorySavePath)\nWrite-Output ('var=[' + $env:WAVETERM_PWSH_HISTFILE + ']')\n"
	out := runPwsh(t, script, PwshHistFileEnvName+"="+file)
	if !strings.Contains(out, "path="+file) {
		t.Fatalf("the save path must be the block's file %q, got:\n%s", file, out)
	}
	if !strings.Contains(out, "var=[]") {
		t.Fatalf("the variable must be removed after use, got:\n%s", out)
	}
}

func TestThePwshStartupLeavesHistoryAloneWhenThereIsNoVariableOrTheEditorFails(t *testing.T) {
	file := filepath.Join(t.TempDir(), "block-history.txt")
	probe := "\nWrite-Output ('path=' + (Get-PSReadLineOption).HistorySavePath)\nWrite-Output 'reached the end'\n"
	// no variable: nothing changes
	out := runPwsh(t, "Import-Module PSReadLine\n"+historyBlock(t)+probe)
	if strings.Contains(out, "block-history") || !strings.Contains(out, "reached the end") {
		t.Fatalf("without the variable the history is untouched, got:\n%s", out)
	}
	// the line editor cannot be loaded: the start goes on, the path is not changed, the variable is still removed
	failing := "function Import-Module { throw 'no line editor' }\n" + historyBlock(t) + probe + "Write-Output ('var=[' + $env:WAVETERM_PWSH_HISTFILE + ']')\n"
	out = runPwsh(t, "Import-Module PSReadLine\n"+failing, PwshHistFileEnvName+"="+file)
	if strings.Contains(out, "block-history") || !strings.Contains(out, "reached the end") || !strings.Contains(out, "var=[]") {
		t.Fatalf("a failing line editor must not break the start or change the path, got:\n%s", out)
	}
}

func TestTheHistoryFileIsReadInPSReadLinesOwnFormat(t *testing.T) {
	file := filepath.Join(t.TempDir(), "h.txt")
	content := "echo one\r\nGet-ChildItem `\r\n  | Select-Object Name `\r\n  | Sort-Object\r\n\r\n   \r\necho two\r\ntrailing `\r\n"
	if err := os.WriteFile(file, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	script := historyBlock(t) + "\n$e = @(_waveterm_history_entries $env:HF)\nWrite-Output ('count=' + $e.Count)\n$e | ForEach-Object { Write-Output ('entry=[' + ($_ -replace \"`n\", '<NL>') + ']') }\n"
	out := runPwsh(t, script, "HF="+file)
	for _, want := range []string{
		"count=4",
		"entry=[echo one]",
		"entry=[Get-ChildItem <NL>  | Select-Object Name <NL>  | Sort-Object]",
		"entry=[echo two]",
		"entry=[trailing ]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestLoadingTheHistoryDoesNotSaveItAgain(t *testing.T) {
	file := filepath.Join(t.TempDir(), "h.txt")
	if err := os.WriteFile(file, []byte("echo one\r\necho two\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	script := historyBlock(t) + `
$log = New-Object System.Collections.Generic.List[string]
_waveterm_load_history $env:HF { param($c) $log.Add('add:' + $c) } { 'SaveIncrementally' } { param($s) $log.Add('style:' + $s) }
Write-Output ($log -join ' | ')
$log.Clear()
try {
    _waveterm_load_history $env:HF { param($c) throw 'add failed' } { 'SaveIncrementally' } { param($s) $log.Add('style:' + $s) }
} catch { }
Write-Output ('after a failure: ' + ($log -join ' | '))
`
	out := runPwsh(t, script, "HF="+file)
	if !strings.Contains(out, "style:SaveNothing | add:echo one | add:echo two | style:SaveIncrementally") {
		t.Errorf("saving must be off while the entries go in and restored after:\n%s", out)
	}
	if !strings.Contains(out, "after a failure: style:SaveNothing | style:SaveIncrementally") {
		t.Errorf("the save style must be restored also when adding fails:\n%s", out)
	}
}

func TestTheHistoryIsOnlyTouchedWhenTheLineEditorCanClearIt(t *testing.T) {
	script := "Import-Module PSReadLine\n" + historyBlock(t) + `
Write-Output ('editor=' + (_waveterm_can_clear_history ([Microsoft.PowerShell.PSConsoleReadLine])))
Write-Output ('plain=' + (_waveterm_can_clear_history ([string])))
`
	out := runPwsh(t, script)
	if !strings.Contains(out, "editor=True") || !strings.Contains(out, "plain=False") {
		t.Fatalf("a type with ClearHistory is true and one without is false, got:\n%s", out)
	}
	if !strings.Contains(historyBlock(t), "if (_waveterm_can_clear_history ([Microsoft.PowerShell.PSConsoleReadLine])) {") {
		t.Fatalf("the block must act only when the check says the editor can clear its history")
	}
}
