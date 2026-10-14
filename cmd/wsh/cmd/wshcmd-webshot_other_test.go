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

// Outside Windows the destination cannot be checked through the new file's own handle, so --out is refused before anything is asked or written.
func TestWebShotIsRefusedOutsideWindowsBeforeAnything(t *testing.T) {
	sent, read := stubWebShot(t)
	out := filepath.Join(t.TempDir(), "a.png")
	webShotOut = out
	err := webShotCmd.RunE(webShotCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "Windows") {
		t.Fatalf("want the Windows-only refusal, got %v", err)
	}
	if *read || len(*sent) != 0 {
		t.Fatal("nothing may be read or sent")
	}
	if _, err := os.Lstat(out); err == nil {
		t.Fatal("no file may be made")
	}
}
