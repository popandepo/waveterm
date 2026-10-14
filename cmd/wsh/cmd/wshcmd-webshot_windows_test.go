// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wavetermdev/waveterm/pkg/wshrpc"
)

// makeJunction makes a real directory junction (mklink /J, which needs no elevation and never makes a symlink).
func makeJunction(t *testing.T, link string, target string) {
	t.Helper()
	out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
	if err != nil {
		t.Fatalf("mklink /J: %v %s", err, out)
	}
	if !strings.Contains(string(out), "Junction created") {
		t.Fatalf("mklink /J did not report a junction: %s", out)
	}
}

func TestWebShotRefusesAJunctionIntoAnOffLimitsFolder(t *testing.T) {
	stubWebShot(t)
	tmp := t.TempDir()
	target := filepath.Join(tmp, ".claude")
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(tmp, "innocent")
	makeJunction(t, link, target)
	// the check itself must see through the junction, not only the write after it
	if _, err := checkWebShotPath(filepath.Join(link, "a.png")); err == nil || !strings.Contains(err.Error(), ".claude") {
		t.Fatalf("the path check must judge the folder the junction leads to, got %v", err)
	}
	webShotOut = filepath.Join(link, "a.png")
	err := webShotCmd.RunE(webShotCmd, nil)
	if err == nil || !strings.Contains(err.Error(), ".claude") {
		t.Fatalf("the folder the junction leads to must be judged, got %v", err)
	}
	if entries, _ := os.ReadDir(target); len(entries) != 0 {
		t.Fatalf("nothing may be written into the off-limits folder: %v", entries)
	}
}

// The folder is checked before the request and the file is made after it. A folder swapped for a junction into .claude in between must not
// receive the picture, and no file may be left anywhere.
func TestWebShotAFolderSwappedForAJunctionDuringTheRequestGetsNothing(t *testing.T) {
	stubWebShot(t)
	tmp := t.TempDir()
	offLimits := filepath.Join(tmp, "home", ".claude-work")
	if err := os.MkdirAll(offLimits, 0700); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(tmp, "out")
	if err := os.MkdirAll(outDir, 0700); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(tmp, "out-moved")
	webShotOut = filepath.Join(outDir, "a.png")
	webShotRPC = func(d wshrpc.CommandWebShotData) (*wshrpc.CommandWebShotRtnData, error) {
		if err := os.Rename(outDir, moved); err != nil {
			t.Fatalf("swap: %v", err)
		}
		makeJunction(t, outDir, offLimits)
		return &wshrpc.CommandWebShotRtnData{Png: pngB64(10), Width: 800, Height: 600}, nil
	}
	err := webShotCmd.RunE(webShotCmd, nil)
	if err == nil {
		t.Fatal("a folder swapped after the check must be refused")
	}
	if entries, _ := os.ReadDir(offLimits); len(entries) != 0 {
		t.Fatalf("the picture reached the off-limits folder: %v", entries)
	}
	if entries, _ := os.ReadDir(moved); len(entries) != 0 {
		t.Fatalf("no file may be left behind: %v", entries)
	}
}

// A folder swapped for a junction to an ordinary other folder is refused too: the file must land in the folder that was checked.
func TestWebShotAFolderSwappedForAJunctionElsewhereGetsNothing(t *testing.T) {
	stubWebShot(t)
	tmp := t.TempDir()
	elsewhere := filepath.Join(tmp, "elsewhere")
	outDir := filepath.Join(tmp, "out")
	for _, d := range []string{elsewhere, outDir} {
		if err := os.MkdirAll(d, 0700); err != nil {
			t.Fatal(err)
		}
	}
	webShotOut = filepath.Join(outDir, "a.png")
	webShotRPC = func(d wshrpc.CommandWebShotData) (*wshrpc.CommandWebShotRtnData, error) {
		if err := os.Rename(outDir, filepath.Join(tmp, "out-moved")); err != nil {
			t.Fatalf("swap: %v", err)
		}
		makeJunction(t, outDir, elsewhere)
		return &wshrpc.CommandWebShotRtnData{Png: pngB64(10), Width: 800, Height: 600}, nil
	}
	if err := webShotCmd.RunE(webShotCmd, nil); err == nil || !strings.Contains(err.Error(), "changed after it was checked") {
		t.Fatalf("want the changed-folder refusal, got %v", err)
	}
	if entries, _ := os.ReadDir(elsewhere); len(entries) != 0 {
		t.Fatalf("no file may be left in the other folder: %v", entries)
	}
}

func TestNormalizeFinalPathKeepsUncAbsolute(t *testing.T) {
	cases := map[string]string{
		`\\?\C:\Users\x\out`:           `C:\Users\x\out`,
		`\\?\UNC\server\share\dir`:     `\\server\share\dir`,
		`\\?\UNC\server\share\.claude`: `\\server\share\.claude`,
		`C:\plain`:                     `C:\plain`,
	}
	for in, want := range cases {
		got := normalizeFinalPath(in)
		if got != want {
			t.Errorf("%s: got %s want %s", in, got, want)
		}
		if !filepath.IsAbs(got) {
			t.Errorf("%s: %s is not absolute", in, got)
		}
	}
}

// A write or a close that fails must not leave a file behind (an empty or half PNG under the name asked for).
func TestWebShotAFailedWriteOrCloseLeavesNoFile(t *testing.T) {
	oldWrite, oldClose := webShotWriteFile, webShotCloseFile
	t.Cleanup(func() { webShotWriteFile, webShotCloseFile = oldWrite, oldClose })
	for _, which := range []string{"write", "close"} {
		stubWebShot(t)
		out := filepath.Join(t.TempDir(), "a.png")
		webShotOut = out
		webShotWriteFile, webShotCloseFile = oldWrite, oldClose
		if which == "write" {
			webShotWriteFile = func(f *os.File, data []byte) error {
				f.Write(data[:3])
				return errString("disk full")
			}
		} else {
			webShotCloseFile = func(f *os.File) error {
				f.Close()
				return errString("close failed")
			}
		}
		if err := webShotCmd.RunE(webShotCmd, nil); err == nil {
			t.Fatalf("%s: the failure must be reported", which)
		}
		if _, err := os.Lstat(out); err == nil {
			t.Fatalf("%s: the file must not be left behind", which)
		}
	}
}
