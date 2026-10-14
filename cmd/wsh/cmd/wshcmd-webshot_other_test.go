// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The folder comes from the user's passwd entry, not from HOME or XDG_CACHE_HOME, which the caller's environment controls.
func TestWebShotFolderIgnoresTheEnvironment(t *testing.T) {
	steer := t.TempDir()
	t.Setenv("HOME", steer)
	t.Setenv("XDG_CACHE_HOME", steer)
	dir, err := defaultWebShotDir()
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(dir, steer) {
		t.Fatalf("the folder followed the environment: %s", dir)
	}
	if !strings.HasSuffix(dir, filepath.Join("Wave", "web-shots")) {
		t.Fatalf("the folder ends in Wave/web-shots, got %s", dir)
	}
}

// A shot folder that is a symlink is refused and nothing is written through it.
func TestWebShotRefusesAShotFolderThatIsASymlink(t *testing.T) {
	stubWebShot(t)
	tmp := t.TempDir()
	target := filepath.Join(tmp, "elsewhere")
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(tmp, "web-shots")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	webShotDir = func() (string, error) { return link, nil }
	if err := webShotCmd.RunE(webShotCmd, nil); err == nil || !strings.Contains(err.Error(), "not a plain folder") {
		t.Fatalf("want the plain-folder refusal, got %v", err)
	}
	if got := filesIn(t, target); len(got) != 0 {
		t.Fatalf("nothing may be written through the link: %v", got)
	}
}
