// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
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
		s := windows.UTF16ToString(buf[:n])
		s = strings.TrimPrefix(s, `\\?\UNC\`)
		if strings.HasPrefix(s, `\\?\`) {
			s = s[4:]
		}
		if s == "" {
			return "", fmt.Errorf("no final path for %s", dir)
		}
		return s, nil
	}
}
