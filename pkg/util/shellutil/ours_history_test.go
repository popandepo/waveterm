// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package shellutil

import (
	"os"
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

func TestThePwshStartupSetsTheHistoryPathFromTheEnvironmentOnly(t *testing.T) {
	script := PwshStartup_wavepwsh
	if !strings.Contains(script, "$env:"+PwshHistFileEnvName) || !strings.Contains(script, "Set-PSReadLineOption -HistorySavePath") {
		t.Fatalf("the startup script must set the history path from %s", PwshHistFileEnvName)
	}
	i := strings.Index(script, "Set-PSReadLineOption -HistorySavePath")
	before := script[:i]
	if !strings.Contains(before[strings.LastIndex(before, "try"):], "try") {
		t.Fatalf("a failure to set the history path must not break the shell start")
	}
	if strings.Contains(script, "Remove-Item") && !strings.Contains(script, "Remove-Item Env:"+PwshHistFileEnvName) {
		t.Fatalf("the variable is removed after use so that child programs do not inherit it")
	}
}
