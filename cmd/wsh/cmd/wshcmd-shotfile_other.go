// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package cmd

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
)

// defaultWebShotDir is Wave/web-shots under the user's cache folder, found from the home folder of the passwd entry of the user running wsh, not
// from HOME or XDG_CACHE_HOME, which the caller's environment sets.
func defaultWebShotDir() (string, error) {
	u, err := user.LookupId(strconv.Itoa(os.Getuid()))
	if err != nil {
		return "", fmt.Errorf("the user's home folder: %w", err)
	}
	if u.HomeDir == "" || !filepath.IsAbs(u.HomeDir) {
		return "", fmt.Errorf("the user's home folder is not known")
	}
	cache := filepath.Join(u.HomeDir, ".cache")
	if runtime.GOOS == "darwin" {
		cache = filepath.Join(u.HomeDir, "Library", "Caches")
	}
	return filepath.Join(cache, "Wave", "web-shots"), nil
}

// checkShotDir refuses a shot folder that is not a plain folder owned by the user running wsh: a symlink could lead anywhere.
func checkShotDir(dir string) error {
	fi, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("the folder for pictures: %w", err)
	}
	if !fi.Mode().IsDir() {
		return fmt.Errorf("the folder for pictures %s is not a plain folder (a link is refused)", dir)
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Getuid() {
		return fmt.Errorf("the folder for pictures %s is not a plain folder of this user", dir)
	}
	return nil
}

// createShotFile makes a new file (O_EXCL: never an existing one).
func createShotFile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
}

// discardShotFile closes and removes the file by name: the folder is the tool's own and the name has a random part, so no other file has it.
func discardShotFile(f *os.File, path string) error {
	f.Close()
	return os.Remove(path)
}

// keepShotFile closes a file whose picture is written and flushed; a close that fails removes it as discardShotFile does.
func keepShotFile(f *os.File, path string) error {
	if err := f.Close(); err != nil {
		if rerr := os.Remove(path); rerr != nil {
			return fmt.Errorf("closing %s: %w (and it could not be removed: %v)", path, err, rerr)
		}
		return fmt.Errorf("closing %s: %w (the file was removed)", path, err)
	}
	return nil
}
