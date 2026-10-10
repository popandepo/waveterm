// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package waveapi

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
)

// chmod does nothing useful on Windows, so access is set with an explicit ACL: inheritance removed,
// the current user the only entry.
func runIcacls(path string, grant string) error {
	cur, err := user.Current()
	if err != nil {
		return fmt.Errorf("finding current user: %w", err)
	}
	out, err := exec.Command("icacls", path, "/inheritance:r", "/grant:r", cur.Username+":"+grant).CombinedOutput()
	if err != nil {
		return fmt.Errorf("icacls %s: %v: %s", path, err, out)
	}
	return nil
}

func PrepareApiDir(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating api folder: %w", err)
	}
	return runIcacls(dir, "(OI)(CI)F")
}

func restrictFile(path string) error {
	return runIcacls(path, "F")
}
