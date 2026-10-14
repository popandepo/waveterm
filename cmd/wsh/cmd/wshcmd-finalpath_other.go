// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// finalFolderPath is the folder that a name really leads to once every link on the way is followed (the Windows version also follows junctions).
func finalFolderPath(dir string) (string, error) {
	return filepath.EvalSymlinks(dir)
}

// createNewFileIn creates path, which must not exist and must not be a link, and then checks that its folder still resolves to wantDir. On a
// mismatch the file is removed by name and nothing is written. (The Windows version checks and removes through the file's own handle.)
func createNewFileIn(path string, wantDir string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	dir, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err == nil && dir == filepath.Clean(wantDir) && !hasOffLimitsSegment(dir) {
		return f, nil
	}
	f.Close()
	os.Remove(path)
	if err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("the folder of --out changed after it was checked (it now leads to %s, not %s)", dir, wantDir)
}
