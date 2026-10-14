// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

// The folder comes from the Known Folder API, not from LOCALAPPDATA, which the caller's environment controls.
func TestWebShotFolderIgnoresTheEnvironment(t *testing.T) {
	steer := t.TempDir()
	t.Setenv("LOCALAPPDATA", steer)
	t.Setenv("USERPROFILE", steer)
	dir, err := defaultWebShotDir()
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(strings.ToLower(dir), strings.ToLower(steer)) {
		t.Fatalf("the folder followed the environment: %s", dir)
	}
	if !strings.HasSuffix(dir, filepath.Join("Wave", "web-shots")) {
		t.Fatalf("the folder is Wave\\web-shots under the local app data folder, got %s", dir)
	}
}

// A shot folder that is a junction (it could lead anywhere) is refused and nothing is written through it.
func TestWebShotRefusesAShotFolderThatIsAJunction(t *testing.T) {
	stubWebShot(t)
	tmp := t.TempDir()
	target := filepath.Join(tmp, "elsewhere")
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(tmp, "web-shots")
	out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
	if err != nil || !strings.Contains(string(out), "Junction created") {
		t.Fatalf("mklink /J: %v %s", err, out)
	}
	webShotDir = func() (string, error) { return link, nil }
	if err := webShotCmd.RunE(webShotCmd, nil); err == nil || !strings.Contains(err.Error(), "not a plain folder") {
		t.Fatalf("want the plain-folder refusal, got %v", err)
	}
	if got := filesIn(t, target); len(got) != 0 {
		t.Fatalf("nothing may be written through the junction: %v", got)
	}
}

// A junction at the Wave folder above web-shots could lead the fixed folder anywhere: refused before anything is made through it.
func TestWebShotRefusesAJunctionAtTheParentFolder(t *testing.T) {
	stubWebShot(t)
	tmp := t.TempDir()
	target := filepath.Join(tmp, "elsewhere")
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	wave := filepath.Join(tmp, "Wave")
	out, err := exec.Command("cmd", "/c", "mklink", "/J", wave, target).CombinedOutput()
	if err != nil || !strings.Contains(string(out), "Junction created") {
		t.Fatalf("mklink /J: %v %s", err, out)
	}
	webShotDir = func() (string, error) { return filepath.Join(wave, "web-shots"), nil }
	if err := webShotCmd.RunE(webShotCmd, nil); err == nil || !strings.Contains(err.Error(), "not a plain folder") {
		t.Fatalf("want the plain-folder refusal for the parent, got %v", err)
	}
	if got := filesIn(t, target); len(got) != 0 {
		t.Fatalf("nothing may be made through the junction, not even web-shots: %v", got)
	}
}

// The new file cannot be marked for removal on failure: it is removed at once, and no empty PNG-named file stays behind.
func TestWebShotAFailedDeletePendingMarkLeavesNoFile(t *testing.T) {
	dir, _, _ := stubWebShot(t)
	old := shotSetDeletePending
	t.Cleanup(func() { shotSetDeletePending = old })
	shotSetDeletePending = func(h windows.Handle, pending bool) error {
		if pending {
			return errString("mark refused")
		}
		return old(h, pending)
	}
	if err := webShotCmd.RunE(webShotCmd, nil); err == nil || !strings.Contains(err.Error(), "mark refused") {
		t.Fatalf("the failure must be reported, got %v", err)
	}
	if got := filesIn(t, dir); len(got) != 0 {
		t.Fatalf("no file may be left behind: %v", got)
	}
}
