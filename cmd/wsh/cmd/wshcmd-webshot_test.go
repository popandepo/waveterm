// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wavetermdev/waveterm/pkg/wshrpc"
)

func pngB64(extra int) string {
	b := append([]byte{}, pngSignature...)
	b = append(b, bytes.Repeat([]byte{1}, extra)...)
	return base64.StdEncoding.EncodeToString(b)
}

// onlyWhereShotRuns skips a test of a working shot outside Windows, where --out is refused (wshcmd-webshot_other_test.go).
func onlyWhereShotRuns(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("wsh web shot --out works on Windows only")
	}
}

func stubWebShot(t *testing.T) (*[]wshrpc.CommandWebShotData, *bool) {
	t.Helper()
	oldAuth, oldInfo, oldRPC, oldResolve, oldOut, oldBlockArg := webNavAuthorize, webNavBlockInfo, webShotRPC, resolveBlockRef, webShotOut, blockArg
	t.Cleanup(func() {
		webNavAuthorize, webNavBlockInfo, webShotRPC, resolveBlockRef, webShotOut, blockArg = oldAuth, oldInfo, oldRPC, oldResolve, oldOut, oldBlockArg
	})
	var sent []wshrpc.CommandWebShotData
	read := false
	webNavAuthorize = func(blockId string) error { return nil }
	webNavBlockInfo = func(blockId string) (*wshrpc.BlockInfoData, error) { read = true; return webInfo("web"), nil }
	resolveBlockRef = func(ref string) (string, error) { return webBlockId, nil }
	webShotRPC = func(d wshrpc.CommandWebShotData) (*wshrpc.CommandWebShotRtnData, error) {
		sent = append(sent, d)
		return &wshrpc.CommandWebShotRtnData{Png: pngB64(10), Width: 800, Height: 600}, nil
	}
	blockArg = ""
	return &sent, &read
}

func TestWebShotWritesANewPngFileAndNeverOverwrites(t *testing.T) {
	onlyWhereShotRuns(t)
	sent, _ := stubWebShot(t)
	out := filepath.Join(t.TempDir(), "tile.png")
	webShotOut = out
	if err := webShotCmd.RunE(webShotCmd, nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil || !bytes.HasPrefix(data, pngSignature) || len(data) != len(pngSignature)+10 {
		t.Fatalf("the file must hold the picture: %v %d", err, len(data))
	}
	if len(*sent) != 1 || (*sent)[0].TabId != "tab-other" || (*sent)[0].BlockId != webBlockId {
		t.Fatalf("bad request: %+v", *sent)
	}
	if err := os.WriteFile(out, []byte("mine"), 0600); err != nil {
		t.Fatal(err)
	}
	*sent = nil
	if err := webShotCmd.RunE(webShotCmd, nil); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("an existing file must be refused, got %v", err)
	}
	if got, _ := os.ReadFile(out); string(got) != "mine" {
		t.Fatalf("the existing file was changed: %q", got)
	}
	if len(*sent) != 0 {
		t.Fatal("a refused path must not ask the window for a picture")
	}
}

func TestWebShotPathIsCheckedBeforeAnyRequest(t *testing.T) {
	onlyWhereShotRuns(t)
	sent, read := stubWebShot(t)
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ".claude"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmp, ".CLAUDE-WORK"), 0700); err != nil {
		t.Fatal(err)
	}
	bad := []string{
		"",
		"   ",
		filepath.Join(tmp, "a.txt"),
		filepath.Join(tmp, "a"),
		filepath.Join(tmp, "missing-folder", "a.png"),
		filepath.Join(tmp, ".claude", "a.png"),
		filepath.Join(tmp, ".CLAUDE-WORK", "a.png"),
		filepath.Join(tmp, ".claude", "deeper", "a.png"),
		filepath.Join(tmp, "a"+string(rune(0))+".png"),
		filepath.Join(tmp, "a"+string(rune(10))+".png"),
	}
	for _, p := range bad {
		webShotOut = p
		if err := webShotCmd.RunE(webShotCmd, nil); err == nil {
			t.Errorf("%q must be refused", p)
		}
	}
	if *read || len(*sent) != 0 {
		t.Fatal("a refused path must not read or send anything")
	}
	webShotOut = filepath.Join(tmp, ".claude-personal-x.png")
	if err := webShotCmd.RunE(webShotCmd, nil); err != nil {
		t.Fatalf("a name that only starts like .claude is fine: %v", err)
	}
}

// A symlinked folder; the Windows junction case is in wshcmd-webshot_windows_test.go.
func TestWebShotRefusesASymlinkedFolderIntoAnOffLimitsFolder(t *testing.T) {
	onlyWhereShotRuns(t)
	stubWebShot(t)
	tmp := t.TempDir()
	target := filepath.Join(tmp, ".claude")
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(tmp, "innocent")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("no symlink could be made (Windows without developer mode): %v", err)
	}
	webShotOut = filepath.Join(link, "a.png")
	err := webShotCmd.RunE(webShotCmd, nil)
	if err == nil || !strings.Contains(err.Error(), ".claude") {
		t.Fatalf("the resolved folder must be judged, got %v", err)
	}
	if entries, _ := os.ReadDir(target); len(entries) != 0 {
		t.Fatalf("nothing may be written into the off-limits folder: %v", entries)
	}
}

func TestWebShotRefusesAForeignTileBeforeAnythingIsReadAndWritesNothing(t *testing.T) {
	onlyWhereShotRuns(t)
	sent, read := stubWebShot(t)
	webNavAuthorize = func(blockId string) error { return errString("no") }
	out := filepath.Join(t.TempDir(), "x.png")
	webShotOut = out
	err := webShotCmd.RunE(webShotCmd, nil)
	if err == nil || err.Error() != webNavDenied {
		t.Fatalf("want the one denial text, got %v", err)
	}
	if *read || len(*sent) != 0 {
		t.Fatal("nothing may be read or sent")
	}
	if _, err := os.Stat(out); err == nil {
		t.Fatal("no file may be made")
	}
}

func TestWebShotTheWindowsRefusalComesBackAndNoFileIsMade(t *testing.T) {
	onlyWhereShotRuns(t)
	stubWebShot(t)
	out := filepath.Join(t.TempDir(), "x.png")
	webShotOut = out
	webShotRPC = func(d wshrpc.CommandWebShotData) (*wshrpc.CommandWebShotRtnData, error) {
		return nil, errString("this web tile uses the shared default sign-in store of Wave, so what it shows is not given out")
	}
	if err := webShotCmd.RunE(webShotCmd, nil); err == nil || !strings.Contains(err.Error(), "shared default sign-in store") {
		t.Fatalf("got %v", err)
	}
	if _, err := os.Stat(out); err == nil {
		t.Fatal("no file may be made when the window refuses")
	}
}

func TestDecodeWebShotRefusesWhatIsNotAPicture(t *testing.T) {
	notPng := base64.StdEncoding.EncodeToString([]byte("GIF89a....."))
	tooBig := pngB64(maxWebShotBytes + 1)
	for name, res := range map[string]*wshrpc.CommandWebShotRtnData{
		"nil":        nil,
		"empty":      {Png: ""},
		"not base64": {Png: "***not base64***"},
		"not a png":  {Png: notPng},
		"too large":  {Png: tooBig},
	} {
		if _, err := decodeWebShot(res); err == nil {
			t.Errorf("%s must be refused", name)
		}
	}
	if data, err := decodeWebShot(&wshrpc.CommandWebShotRtnData{Png: pngB64(5)}); err != nil || len(data) != len(pngSignature)+5 {
		t.Fatalf("a small png must pass: %v", err)
	}
}
