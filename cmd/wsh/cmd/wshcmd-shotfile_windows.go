// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"os"

	"golang.org/x/sys/windows"
)

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
