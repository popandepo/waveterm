// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package waveapi

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func testUserSid(t *testing.T) *windows.SID {
	t.Helper()
	sid, err := currentUserSid()
	if err != nil {
		t.Fatal(err)
	}
	return sid
}

// "exactly one entry, this user", read from the security descriptor and not from the text of a tool
func requireOwnerOnly(t *testing.T, path string, isDir bool) {
	t.Helper()
	sid := testUserSid(t)
	info, err := DescribeAcl(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Owner != sid.String() {
		t.Fatalf("%s: owner %s, want %s", path, info.Owner, sid.String())
	}
	if len(info.Aces) != 1 {
		t.Fatalf("%s: %d entries: %+v", path, len(info.Aces), info.Aces)
	}
	ace := info.Aces[0]
	if !ace.Allow || ace.Sid != sid.String() || ace.Mask&fileAllAccess != fileAllAccess {
		t.Fatalf("%s: the only entry is %+v", path, ace)
	}
	if isDir && !info.Protected {
		t.Fatalf("%s: still inherits from its parent", path)
	}
}

func TestFreshApiFolderAndTokenFileAreOwnerOnly(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "api")
	if err := PrepareApiDir(dir); err != nil {
		t.Fatal(err)
	}
	requireOwnerOnly(t, dir, true)
	if err := WriteTokenFile(dir, testToken); err != nil {
		t.Fatal(err)
	}
	requireOwnerOnly(t, filepath.Join(dir, TokenFileName), false)
}

func TestExistingFolderWithBroadEntriesEndsOwnerOnly(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "api")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	broad, err := windows.SecurityDescriptorFromString("D:(A;OICI;FR;;;WD)(A;OICI;FR;;;BU)(A;OICI;FR;;;AU)(A;OICI;FR;;;IU)")
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, _ := broad.DACL()
	if err := windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		t.Fatal(err)
	}
	before, _ := DescribeAcl(dir)
	if len(before.Aces) < 4 {
		t.Fatalf("test setup did not take: %+v", before)
	}
	if err := PrepareApiDir(dir); err != nil {
		t.Fatal(err)
	}
	requireOwnerOnly(t, dir, true)
}

func TestJunctionAtTheApiPathIsRefused(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "elsewhere")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "api")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
		t.Fatalf("mklink: %v: %s", err, out)
	}
	if err := PrepareApiDir(link); err == nil {
		t.Fatalf("a junction was accepted as the api folder")
	}
}

func TestCheckSecurityDescriptor(t *testing.T) {
	sid := testUserSid(t)
	s := sid.String()
	cases := []struct {
		name  string
		sddl  string
		isDir bool
		ok    bool
	}{
		{"owner only folder", "O:" + s + "D:P(A;OICI;FA;;;" + s + ")", true, true},
		{"owner only file", "O:" + s + "D:(A;;FA;;;" + s + ")", false, true},
		{"owner is someone else", "O:BAD:P(A;OICI;FA;;;" + s + ")", true, false},
		{"second entry for everyone", "O:" + s + "D:P(A;OICI;FA;;;" + s + ")(A;OICI;FR;;;WD)", true, false},
		{"only entry is for another sid", "O:" + s + "D:P(A;OICI;FA;;;BA)", true, false},
		{"deny entry", "O:" + s + "D:P(D;OICI;FA;;;" + s + ")", true, false},
		{"folder not protected", "O:" + s + "D:(A;OICI;FA;;;" + s + ")", true, false},
		{"folder entry not inherited", "O:" + s + "D:P(A;;FA;;;" + s + ")", true, false},
		{"read only", "O:" + s + "D:P(A;OICI;FR;;;" + s + ")", true, false},
		{"empty dacl", "O:" + s + "D:P", true, false},
		{"no dacl", "O:" + s, false, false},
	}
	for _, c := range cases {
		sd, err := windows.SecurityDescriptorFromString(c.sddl)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		err = checkSecurityDescriptor(sd, sid, c.isDir)
		if c.ok && err != nil {
			t.Fatalf("%s: should pass, got %v", c.name, err)
		}
		if !c.ok && err == nil {
			t.Fatalf("%s: should be refused", c.name)
		}
	}
}

func TestVerifyPrivateRefusesAFolderThatInheritsBroadEntries(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "plain")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := VerifyPrivate(dir, true); err == nil {
		t.Fatalf("a folder that inherits the parent's entries passed")
	}
}
