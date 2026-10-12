// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package cmd

import "path/filepath"

// finalFolderPath is the folder that a name really leads to once every link on the way is followed (the Windows version also follows junctions).
func finalFolderPath(dir string) (string, error) {
	return filepath.EvalSymlinks(dir)
}
