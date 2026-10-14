// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/wavetermdev/waveterm/pkg/wshrpc"
)

// realPNG is a complete PNG of w by h pixels.
func realPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 200, A: 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// stubWebShot points the shot folder at a fresh temp folder (not yet created) and answers every request with a real 4x3 PNG.
func stubWebShot(t *testing.T) (dir string, sent *[]wshrpc.CommandWebShotData, read *bool) {
	t.Helper()
	oldAuth, oldInfo, oldRPC, oldResolve, oldBlockArg, oldDir := webNavAuthorize, webNavBlockInfo, webShotRPC, resolveBlockRef, blockArg, webShotDir
	oldOut, oldErr := WrappedStdout, WrappedStderr
	t.Cleanup(func() {
		webNavAuthorize, webNavBlockInfo, webShotRPC, resolveBlockRef, blockArg, webShotDir = oldAuth, oldInfo, oldRPC, oldResolve, oldBlockArg, oldDir
		WrappedStdout, WrappedStderr = oldOut, oldErr
	})
	var s []wshrpc.CommandWebShotData
	r := false
	dir = filepath.Join(t.TempDir(), "web-shots")
	webShotDir = func() (string, error) { return dir, nil }
	webNavAuthorize = func(blockId string) error { return nil }
	webNavBlockInfo = func(blockId string) (*wshrpc.BlockInfoData, error) { r = true; return webInfo("web"), nil }
	resolveBlockRef = func(ref string) (string, error) { return webBlockId, nil }
	pic := base64.StdEncoding.EncodeToString(realPNG(t, 4, 3))
	webShotRPC = func(d wshrpc.CommandWebShotData) (*wshrpc.CommandWebShotRtnData, error) {
		s = append(s, d)
		return &wshrpc.CommandWebShotRtnData{Png: pic, Width: 4, Height: 3}, nil
	}
	blockArg = ""
	WrappedStdout, WrappedStderr = &bytes.Buffer{}, &bytes.Buffer{}
	return dir, &s, &r
}

func filesIn(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestWebShotHasNoOutFlag(t *testing.T) {
	if webShotCmd.Flags().Lookup("out") != nil {
		t.Fatal("the caller must not choose where the picture goes")
	}
	if err := webShotCmd.Args(webShotCmd, []string{"x.png"}); err == nil {
		t.Fatal("the command takes no arguments, so no path either")
	}
}

func TestWebShotWritesANewFileWithAGeneratedNameInItsOwnFolder(t *testing.T) {
	dir, sent, _ := stubWebShot(t)
	var out bytes.Buffer
	WrappedStdout = &out
	if err := webShotCmd.RunE(webShotCmd, nil); err != nil {
		t.Fatal(err)
	}
	names := filesIn(t, dir)
	if len(names) != 1 {
		t.Fatalf("want one file in the shot folder (made by the tool), got %v", names)
	}
	re := regexp.MustCompile(`^` + regexp.QuoteMeta(webBlockId) + `-\d{8}T\d{6}Z-[0-9a-f]{8}\.png$`)
	if !re.MatchString(names[0]) {
		t.Fatalf("the name is <blockid>-<utc time>-<random>.png, got %s", names[0])
	}
	path := filepath.Join(dir, names[0])
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		t.Fatalf("the file must hold the picture: %v", err)
	}
	if !strings.HasPrefix(out.String(), path+" ") {
		t.Fatalf("the path is printed: %q", out.String())
	}
	if len(*sent) != 1 || (*sent)[0].TabId != "tab-other" || (*sent)[0].BlockId != webBlockId {
		t.Fatalf("bad request: %+v", *sent)
	}
	// a second shot is a second file
	if err := webShotCmd.RunE(webShotCmd, nil); err != nil {
		t.Fatal(err)
	}
	if got := filesIn(t, dir); len(got) != 2 {
		t.Fatalf("two shots, two files: %v", got)
	}
}

func TestNewShotFileNeverOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.png")
	if err := os.WriteFile(path, []byte("mine"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeNewShotFile(path, realPNG(t, 2, 2)); err == nil {
		t.Fatal("an existing file must not be written over")
	}
	if data, _ := os.ReadFile(path); string(data) != "mine" {
		t.Fatal("the existing file was changed")
	}
}

func TestWebShotRefusesAForeignTileBeforeAnythingIsReadAndWritesNothing(t *testing.T) {
	dir, sent, read := stubWebShot(t)
	webNavAuthorize = func(blockId string) error { return errString("no") }
	err := webShotCmd.RunE(webShotCmd, nil)
	if err == nil || err.Error() != webNavDenied {
		t.Fatalf("want the one denial text, got %v", err)
	}
	if *read || len(*sent) != 0 {
		t.Fatal("nothing may be read or sent")
	}
	if got := filesIn(t, dir); len(got) != 0 {
		t.Fatalf("no file may be made: %v", got)
	}
}

func TestWebShotTheWindowsRefusalComesBackAndNoFileIsMade(t *testing.T) {
	dir, _, _ := stubWebShot(t)
	webShotRPC = func(d wshrpc.CommandWebShotData) (*wshrpc.CommandWebShotRtnData, error) {
		return nil, errString("this web tile uses the shared default sign-in store of Wave, so what it shows is not given out")
	}
	if err := webShotCmd.RunE(webShotCmd, nil); err == nil || !strings.Contains(err.Error(), "shared default sign-in store") {
		t.Fatalf("got %v", err)
	}
	if got := filesIn(t, dir); len(got) != 0 {
		t.Fatalf("no file may be made when the window refuses: %v", got)
	}
}

// Only a complete PNG is written: the signature alone, a cut picture or other bytes are refused before any file exists.
func TestWebShotWritesOnlyACompletePng(t *testing.T) {
	good := realPNG(t, 4, 3)
	for name, data := range map[string][]byte{
		"signature only":     append([]byte{}, pngSignature...),
		"signature and junk": append(append([]byte{}, pngSignature...), bytes.Repeat([]byte{1}, 40)...),
		"cut in half":        good[:len(good)/2],
		"gif":                []byte("GIF89a....."),
	} {
		dir, _, _ := stubWebShot(t)
		b64 := base64.StdEncoding.EncodeToString(data)
		webShotRPC = func(d wshrpc.CommandWebShotData) (*wshrpc.CommandWebShotRtnData, error) {
			return &wshrpc.CommandWebShotRtnData{Png: b64, Width: 4, Height: 3}, nil
		}
		if err := webShotCmd.RunE(webShotCmd, nil); err == nil {
			t.Errorf("%s must be refused", name)
		}
		if got := filesIn(t, dir); len(got) != 0 {
			t.Errorf("%s: no file may be made: %v", name, got)
		}
	}
}

func TestDecodeWebShotRefusesWhatIsNotAPicture(t *testing.T) {
	tooBig := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, maxWebShotBytes+1))
	for name, res := range map[string]*wshrpc.CommandWebShotRtnData{
		"nil":        nil,
		"empty":      {Png: ""},
		"not base64": {Png: "***not base64***"},
		"too large":  {Png: tooBig},
	} {
		if _, err := decodeWebShot(res); err == nil {
			t.Errorf("%s must be refused", name)
		}
	}
	good := realPNG(t, 2, 2)
	if data, err := decodeWebShot(&wshrpc.CommandWebShotRtnData{Png: base64.StdEncoding.EncodeToString(good)}); err != nil || !bytes.Equal(data, good) {
		t.Fatalf("a small complete png must pass: %v", err)
	}
}

// A write that fails must not leave a file behind.
func TestWebShotAFailedWriteLeavesNoFile(t *testing.T) {
	dir, _, _ := stubWebShot(t)
	old := webShotWriteFile
	t.Cleanup(func() { webShotWriteFile = old })
	webShotWriteFile = func(f *os.File, data []byte) error {
		f.Write(data[:3])
		return errString("disk full")
	}
	if err := webShotCmd.RunE(webShotCmd, nil); err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("the failure must be reported, got %v", err)
	}
	if got := filesIn(t, dir); len(got) != 0 {
		t.Fatalf("the file must not be left behind: %v", got)
	}
}

// pngDeclaring is a small valid PNG whose header claims w by h pixels (the IHDR is rewritten and its CRC fixed), the shape of a decompression bomb.
func pngDeclaring(t *testing.T, w, h uint32) []byte {
	t.Helper()
	data := realPNG(t, 1, 1)
	// signature (8), IHDR length (4), "IHDR" (4), then width and height
	binary.BigEndian.PutUint32(data[16:20], w)
	binary.BigEndian.PutUint32(data[20:24], h)
	binary.BigEndian.PutUint32(data[29:33], crc32.ChecksumIEEE(data[12:29]))
	return data
}

func TestWebShotRefusesHugeDeclaredDimensionsBeforeDecoding(t *testing.T) {
	for _, wh := range [][2]uint32{{60000, 60000}, {8193, 10}, {10, 8193}, {5000, 4000}} {
		bomb := pngDeclaring(t, wh[0], wh[1])
		if len(bomb) > 200 {
			t.Fatalf("the test picture must be tiny, got %d bytes", len(bomb))
		}
		_, err := decodeWebShot(&wshrpc.CommandWebShotRtnData{Png: base64.StdEncoding.EncodeToString(bomb)})
		if err == nil || !strings.Contains(err.Error(), "pixels") {
			t.Fatalf("%dx%d must be refused on its declared size, got %v", wh[0], wh[1], err)
		}
	}
	if _, err := decodeWebShot(&wshrpc.CommandWebShotRtnData{Png: base64.StdEncoding.EncodeToString(realPNG(t, 64, 48))}); err != nil {
		t.Fatalf("an ordinary picture passes: %v", err)
	}
}

// Only the newest webShotKeep pictures stay; older ones named like ours are removed, anything else in the folder is left alone.
func TestWebShotKeepsTheNewestPicturesOnly(t *testing.T) {
	dir, _, _ := stubWebShot(t)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var old []string
	for i := 0; i < webShotKeep+5; i++ {
		name := fmt.Sprintf("%s-%s-%08x.png", webBlockId, base.Add(time.Duration(i)*time.Minute).Format("20060102T150405Z"), i)
		old = append(old, name)
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, other := range []string{"notes.txt", "mine.png"} {
		if err := os.WriteFile(filepath.Join(dir, other), []byte("keep"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := webShotCmd.RunE(webShotCmd, nil); err != nil {
		t.Fatal(err)
	}
	names := filesIn(t, dir)
	have := map[string]bool{}
	ours := 0
	for _, n := range names {
		have[n] = true
		if webShotNameRe.MatchString(n) {
			ours++
		}
	}
	if ours != webShotKeep {
		t.Fatalf("want %d pictures kept, got %d", webShotKeep, ours)
	}
	if !have["notes.txt"] || !have["mine.png"] {
		t.Fatal("files not named like ours stay")
	}
	for _, n := range old[:6] {
		if have[n] {
			t.Fatalf("the oldest pictures go first: %s is still there", n)
		}
	}
	if !have[old[len(old)-1]] {
		t.Fatal("the newest old picture stays")
	}
}

// A flush that fails after the write must not leave a file behind either.
func TestWebShotAFailedFlushLeavesNoFile(t *testing.T) {
	dir, _, _ := stubWebShot(t)
	old := webShotSyncFile
	t.Cleanup(func() { webShotSyncFile = old })
	webShotSyncFile = func(f *os.File) error { return errString("flush failed") }
	if err := webShotCmd.RunE(webShotCmd, nil); err == nil || !strings.Contains(err.Error(), "flush failed") {
		t.Fatalf("the failure must be reported, got %v", err)
	}
	if got := filesIn(t, dir); len(got) != 0 {
		t.Fatalf("the file must not be left behind: %v", got)
	}
}
