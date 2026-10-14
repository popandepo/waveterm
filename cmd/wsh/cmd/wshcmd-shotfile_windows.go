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

func setDeletePending(h windows.Handle, pending bool) error {
	return shotSetDeletePending(h, pending)
}

// seam for tests
var shotSetDeletePending = func(h windows.Handle, pending bool) error {
	v := byte(0)
	if pending {
		v = 1
	}
	return windows.SetFileInformationByHandle(h, windows.FileDispositionInfo, &v, 1)
}

// createShotFile makes a new file (CREATE_NEW: never an existing one) opened with the right to delete it, and marks it delete-pending at once: it
// disappears when its handle closes unless keepShotFile clears the mark after the whole picture is written and flushed.
func createShotFile(path string) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_WRITE|windows.DELETE, 0, nil, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, &os.PathError{Op: "create", Path: path, Err: err}
	}
	if err := setDeletePending(h, true); err != nil {
		// the mark is not there, so the close keeps the (empty) file: it is removed by name, in the tool's own folder under a random name
		windows.CloseHandle(h)
		if rerr := os.Remove(path); rerr != nil {
			return nil, fmt.Errorf("the new file %s cannot be marked for removal on failure: %w (and it could not be removed: %v)", path, err, rerr)
		}
		return nil, fmt.Errorf("the new file %s cannot be marked for removal on failure, so it was removed: %w", path, err)
	}
	return os.NewFile(uintptr(h), path), nil
}

// discardShotFile closes the delete-pending file, which removes it, and makes sure the mark is set first.
func discardShotFile(f *os.File, path string) error {
	err := setDeletePending(windows.Handle(f.Fd()), true)
	cerr := f.Close()
	if err != nil {
		return err
	}
	return cerr
}

// keepShotFile clears the delete-pending mark of a file whose picture is written and flushed, and closes it. If the mark cannot be cleared the
// close removes the file, and that is reported.
func keepShotFile(f *os.File, path string) error {
	if err := setDeletePending(windows.Handle(f.Fd()), false); err != nil {
		f.Close()
		return fmt.Errorf("keeping %s: %w (the file was removed)", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing %s: %w (the picture was written and flushed before the close)", path, err)
	}
	return nil
}
