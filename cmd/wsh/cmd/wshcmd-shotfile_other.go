// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package cmd

import "os"

// createShotFile makes a new file (O_EXCL: never an existing one).
func createShotFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
}

// discardShotFile closes and removes the file by name: the folder is the tool's own and the name has a random part, so no other file has it.
func discardShotFile(f *os.File, path string) error {
	f.Close()
	return os.Remove(path)
}
