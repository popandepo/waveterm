// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// finalFolderPath is the folder that a name really leads to once every link, junction and mount point on the way is followed: what Windows itself
// reports for an open handle of the folder. filepath.EvalSymlinks does not follow a junction, and a junction named innocent that points into a
// .claude folder must be judged as the folder it is.
func finalFolderPath(dir string) (string, error) {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return "", err
	}
	h, err := windows.CreateFile(p, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)
	s, err := finalPathOfHandle(h)
	if err != nil {
		return "", fmt.Errorf("no final path for %s: %w", dir, err)
	}
	return s, nil
}

func finalPathOfHandle(h windows.Handle) (string, error) {
	buf := make([]uint16, 1024)
	for {
		n, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), 0)
		if err != nil {
			return "", err
		}
		if int(n) > len(buf) {
			buf = make([]uint16, n+1)
			continue
		}
		s := normalizeFinalPath(windows.UTF16ToString(buf[:n]))
		if s == "" {
			return "", fmt.Errorf("empty final path")
		}
		return s, nil
	}
}

// normalizeFinalPath turns what GetFinalPathNameByHandle reports into an ordinary path: \\?\C:\x is C:\x and \\?\UNC\server\share\x is
// \\server\share\x (cutting only \\?\UNC\ would leave server\share\x, a relative path).
func normalizeFinalPath(s string) string {
	if strings.HasPrefix(s, `\\?\UNC\`) {
		return `\\` + s[len(`\\?\UNC\`):]
	}
	return strings.TrimPrefix(s, `\\?\`)
}

// createNewFileIn creates path, which must not exist, and then asks the new file's own handle where it really is. If its folder is not wantDir (a
// folder or junction on the way was swapped after the check) or it is inside an off-limits folder, the file is deleted through that same handle,
// so the name cannot be raced a second time, and nothing is written.
func createNewFileIn(path string, wantDir string) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_WRITE|windows.DELETE, 0, nil, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, err
	}
	final, err := finalPathOfHandle(h)
	if err == nil && strings.EqualFold(filepath.Dir(final), filepath.Clean(wantDir)) && !hasOffLimitsSegment(final) {
		return os.NewFile(uintptr(h), path), nil
	}
	deleteFile := byte(1)
	derr := windows.SetFileInformationByHandle(h, windows.FileDispositionInfo, &deleteFile, 1)
	windows.CloseHandle(h)
	if err != nil {
		return nil, fmt.Errorf("the new file cannot be located (removed: %v): %w", derr == nil, err)
	}
	return nil, fmt.Errorf("the folder of --out changed after it was checked (the file landed in %s, not %s; removed: %v)", filepath.Dir(final), wantDir, derr == nil)
}
