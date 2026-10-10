// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package waveapi

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	fileAllAccess = 0x1F01FF
	inheritedAce  = 0x10
)

// No child process is started: a folder that is looked up by name on PATH could be run by anyone who can
// write to a folder on PATH. The folder is created with its final security descriptor in one call, so
// there is no moment with an inherited or an empty ACL.
func currentUserSid() (*windows.SID, error) {
	tokenUser, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, fmt.Errorf("reading the current user: %w", err)
	}
	return tokenUser.User.Sid, nil
}

func ownerOnlySddl(sid *windows.SID) string {
	s := sid.String()
	return "O:" + s + "D:P(A;OICI;FA;;;" + s + ")"
}

func PrepareApiDir(dir string) error {
	sid, err := currentUserSid()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0700); err != nil {
		return fmt.Errorf("creating parent of api folder: %w", err)
	}
	sd, err := windows.SecurityDescriptorFromString(ownerOnlySddl(sid))
	if err != nil {
		return fmt.Errorf("building security descriptor: %w", err)
	}
	pathPtr, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return err
	}
	sa := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	err = windows.CreateDirectory(pathPtr, sa)
	if err == windows.ERROR_ALREADY_EXISTS {
		err = lockExistingDir(dir, sid, sd)
	}
	if err != nil {
		return fmt.Errorf("locking api folder: %w", err)
	}
	return VerifyPrivate(dir, true)
}

// An existing folder is only taken over when the current user owns it and it is a plain folder: the
// owner of an object can rewrite its ACL whatever the entries say, and a junction would lock some other place.
func lockExistingDir(dir string, sid *windows.SID, sd *windows.SECURITY_DESCRIPTOR) error {
	fi, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if fi.Mode().Type() != fs.ModeDir {
		return fmt.Errorf("%s is not a plain folder", dir)
	}
	old, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	owner, _, err := old.Owner()
	if err != nil || owner == nil || !windows.EqualSid(owner, sid) {
		return fmt.Errorf("%s is owned by someone else", dir)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}

// VerifyPrivate reads the owner and the DACL back from the object and refuses unless the owner is the
// current user and the DACL is exactly one allow entry for that user with full control.
func VerifyPrivate(path string, isDir bool) error {
	sid, err := currentUserSid()
	if err != nil {
		return err
	}
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("reading security of %s: %w", path, err)
	}
	if err := checkSecurityDescriptor(sd, sid, isDir); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func checkSecurityDescriptor(sd *windows.SECURITY_DESCRIPTOR, sid *windows.SID, isDir bool) error {
	owner, _, err := sd.Owner()
	if err != nil || owner == nil {
		return fmt.Errorf("no owner")
	}
	if !windows.EqualSid(owner, sid) {
		return fmt.Errorf("owner is %s, not the current user", owner.String())
	}
	control, _, err := sd.Control()
	if err != nil {
		return err
	}
	if isDir && control&windows.SE_DACL_PROTECTED == 0 {
		return fmt.Errorf("the folder still inherits entries from its parent")
	}
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil {
		return fmt.Errorf("no DACL (a missing DACL grants everyone access)")
	}
	if dacl.AceCount != 1 {
		return fmt.Errorf("DACL has %d entries, want exactly 1", dacl.AceCount)
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(dacl, 0, &ace); err != nil {
		return err
	}
	if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
		return fmt.Errorf("the only entry is not an allow entry")
	}
	aceSid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	if !windows.EqualSid(aceSid, sid) {
		return fmt.Errorf("the only entry is for %s, not the current user", aceSid.String())
	}
	if ace.Mask&fileAllAccess != fileAllAccess {
		return fmt.Errorf("the entry is not full control")
	}
	if isDir && ace.Header.AceFlags&(windows.OBJECT_INHERIT_ACE|windows.CONTAINER_INHERIT_ACE) != (windows.OBJECT_INHERIT_ACE|windows.CONTAINER_INHERIT_ACE) {
		return fmt.Errorf("the entry is not inherited by children")
	}
	return nil
}

type AceInfo struct {
	Sid       string
	Allow     bool
	Mask      uint32
	Inherited bool
}

type AclInfo struct {
	Owner     string
	Protected bool
	Aces      []AceInfo
}

// DescribeAcl is for tests: it reads the security descriptor, not the text of a tool.
func DescribeAcl(path string) (AclInfo, error) {
	var info AclInfo
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return info, err
	}
	owner, _, err := sd.Owner()
	if err != nil || owner == nil {
		return info, fmt.Errorf("no owner")
	}
	info.Owner = owner.String()
	control, _, err := sd.Control()
	if err != nil {
		return info, err
	}
	info.Protected = control&windows.SE_DACL_PROTECTED != 0
	dacl, _, err := sd.DACL()
	if err != nil || dacl == nil {
		return info, fmt.Errorf("no DACL")
	}
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil {
			return info, err
		}
		info.Aces = append(info.Aces, AceInfo{
			Sid:       (*windows.SID)(unsafe.Pointer(&ace.SidStart)).String(),
			Allow:     ace.Header.AceType == windows.ACCESS_ALLOWED_ACE_TYPE,
			Mask:      uint32(ace.Mask),
			Inherited: ace.Header.AceFlags&inheritedAce != 0,
		})
	}
	return info, nil
}

// A socket file created inside the locked folder inherits its single entry.
func restrictSocket(path string) error {
	return nil
}
