// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// defaultWebShotDir is AppData\Local\Wave\web-shots under the profile folder of the user running wsh, taken from the process token (the profile
// list in the registry). LOCALAPPDATA and USERPROFILE come from the caller's environment, and the Known Folder API expands USERPROFILE from it
// too, so neither is used.
func defaultWebShotDir() (string, error) {
	profile, err := windows.GetCurrentProcessToken().GetUserProfileDirectory()
	if err != nil {
		return "", fmt.Errorf("the user's profile folder: %w", err)
	}
	return filepath.Join(profile, "AppData", "Local", "Wave", "web-shots"), nil
}

// checkShotDir refuses a shot folder that is not a plain folder: a junction, a symlink or any other reparse point could lead anywhere.
func checkShotDir(dir string) error {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return err
	}
	attrs, err := windows.GetFileAttributes(p)
	if err != nil {
		return fmt.Errorf("the folder for pictures: %w", err)
	}
	if attrs&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fmt.Errorf("the folder for pictures %s is not a plain folder (a junction or link is refused)", dir)
	}
	return nil
}

// createShotFile makes a new file (CREATE_NEW: never an existing one) opened with the right to delete it, so a failed write can remove it through
// this same handle.
func createShotFile(path string) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_WRITE|windows.DELETE, 0, nil, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, &os.PathError{Op: "create", Path: path, Err: err}
	}
	return os.NewFile(uintptr(h), path), nil
}

// discardShotFile marks the file for deletion through its own handle and closes it; the deletion happens on that close.
func discardShotFile(f *os.File, path string) error {
	deleteFile := byte(1)
	err := windows.SetFileInformationByHandle(windows.Handle(f.Fd()), windows.FileDispositionInfo, &deleteFile, 1)
	cerr := f.Close()
	if err != nil {
		return err
	}
	return cerr
}
