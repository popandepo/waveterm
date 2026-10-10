// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package waveapi

import (
	"fmt"
	"os"
)

func PrepareApiDir(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating api folder: %w", err)
	}
	return os.Chmod(dir, 0700)
}

func restrictFile(path string) error {
	return os.Chmod(path, 0600)
}
