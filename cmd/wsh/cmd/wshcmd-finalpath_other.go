// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
)

// finalFolderPath is the folder that a name really leads to once every link on the way is followed (the Windows version also follows junctions).
func finalFolderPath(dir string) (string, error) {
	return filepath.EvalSymlinks(dir)
}

// webShotPlatformRefusal refuses wsh web shot outside Windows: there the destination folder can be swapped after it is resolved and before the file
// is made, and a removal by name could hit another file. Windows checks the new file through its own handle.
func webShotPlatformRefusal() error {
	return fmt.Errorf("wsh web shot --out works on Windows only (the destination cannot be checked safely here)")
}

func createNewFileIn(path string, wantDir string) (*os.File, error) {
	return nil, webShotPlatformRefusal()
}

func discardNewFile(f *os.File) {
	f.Close()
}
