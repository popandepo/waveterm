// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package waveapi

import (
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

func ownedByCurrentUser(fi fs.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == os.Getuid()
}

func PrepareApiDir(dir string) error {
	if fi, err := os.Lstat(dir); err == nil {
		if fi.Mode().Type() != fs.ModeDir {
			return fmt.Errorf("%s is not a plain folder", dir)
		}
		if !ownedByCurrentUser(fi) {
			return fmt.Errorf("%s is owned by someone else", dir)
		}
	} else if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating api folder: %w", err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return fmt.Errorf("locking api folder: %w", err)
	}
	return VerifyPrivate(dir, true)
}

// VerifyPrivate refuses unless the current user owns the path and nobody else has any access bit.
func VerifyPrivate(path string, isDir bool) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !ownedByCurrentUser(fi) {
		return fmt.Errorf("%s is owned by someone else", path)
	}
	if fi.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("%s is accessible to others (mode %v)", path, fi.Mode().Perm())
	}
	return nil
}

func restrictSocket(path string) error {
	return os.Chmod(path, 0600)
}
