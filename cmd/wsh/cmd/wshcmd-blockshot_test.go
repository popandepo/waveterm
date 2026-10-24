// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wavetermdev/waveterm/pkg/wshrpc"
)

// stubBlockShot points the shot folder at a fresh temp folder, makes every block a terminal in tab "tab-other" of workspace "ws-1", and answers
// every request with a real 4x3 PNG. order records the calls in the order they happened.
func stubBlockShot(t *testing.T) (dir string, sent *[]wshrpc.CommandBlockShotData, order *[]string) {
	t.Helper()
	oldAuth, oldInfo, oldRPC, oldResolve, oldBlockArg, oldDir := webNavAuthorize, webNavBlockInfo, blockShotRPC, resolveBlockRef, blockArg, webShotDir
	oldOut, oldErr := WrappedStdout, WrappedStderr
	t.Cleanup(func() {
		webNavAuthorize, webNavBlockInfo, blockShotRPC, resolveBlockRef, blockArg, webShotDir = oldAuth, oldInfo, oldRPC, oldResolve, oldBlockArg, oldDir
		WrappedStdout, WrappedStderr = oldOut, oldErr
	})
	var s []wshrpc.CommandBlockShotData
	var o []string
	dir = filepath.Join(t.TempDir(), "web-shots")
	webShotDir = func() (string, error) { return dir, nil }
	webNavAuthorize = func(blockId string) error { o = append(o, "authorize"); return nil }
	webNavBlockInfo = func(blockId string) (*wshrpc.BlockInfoData, error) { o = append(o, "info"); return webInfo("term"), nil }
	resolveBlockRef = func(ref string) (string, error) { return webBlockId, nil }
	pic := base64.StdEncoding.EncodeToString(realPNG(t, 4, 3))
	blockShotRPC = func(d wshrpc.CommandBlockShotData) (*wshrpc.CommandBlockShotRtnData, error) {
		o = append(o, "rpc")
		s = append(s, d)
		return &wshrpc.CommandBlockShotRtnData{Png: pic, Width: 4, Height: 3}, nil
	}
	blockArg = ""
	WrappedStdout, WrappedStderr = &bytes.Buffer{}, &bytes.Buffer{}
	return dir, &s, &o
}

func TestBlockShotHasNoOutFlagAndNoArguments(t *testing.T) {
	if blockShotCmd.Flags().Lookup("out") != nil {
		t.Fatal("the caller must not choose where the picture goes")
	}
	if err := blockShotCmd.Args(blockShotCmd, []string{"x.png"}); err == nil {
		t.Fatal("the command takes no arguments, so no path either")
	}
}

func TestBlockShotAsksTheServerFirstThenTakesTheIdsFromTheBlockRecordForAnyView(t *testing.T) {
	dir, sent, order := stubBlockShot(t)
	var out bytes.Buffer
	WrappedStdout = &out
	if err := blockShotCmd.RunE(blockShotCmd, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Join(*order, ",") != "authorize,info,rpc" {
		t.Fatalf("the server is asked first, then the record, then the window: %v", *order)
	}
	if len(*sent) != 1 || (*sent)[0].BlockId != webBlockId || (*sent)[0].TabId != "tab-other" || (*sent)[0].WorkspaceId != "ws-1" {
		t.Fatalf("the ids come from the block record: %+v", *sent)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("want one new file in the shot folder: %v %v", entries, err)
	}
	data, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		t.Fatalf("the file must hold the picture: %v", err)
	}
	if !strings.HasPrefix(out.String(), filepath.Join(dir, entries[0].Name())+" ") {
		t.Fatalf("the path is printed first: %q", out.String())
	}
}

func TestBlockShotRefusedCallerSeesTheSameWordsAsAnUnknownBlockAndNothingIsAskedOrWritten(t *testing.T) {
	dir, sent, order := stubBlockShot(t)
	webNavAuthorize = func(blockId string) error { *order = append(*order, "authorize"); return errors.New("secret detail about another workspace") }
	err := blockShotCmd.RunE(blockShotCmd, nil)
	if err == nil || err.Error() != blockShotDenied {
		t.Fatalf("want the one denial text, got %v", err)
	}
	if len(*sent) != 0 || strings.Join(*order, ",") != "authorize" {
		t.Fatalf("nothing after the refusal: %v %v", *order, *sent)
	}
	if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
		t.Fatal("no folder or file is made for a refused caller")
	}
}

func TestBlockShotRefusesAnAnswerThatIsNotAPictureAndWritesNothing(t *testing.T) {
	dir, _, _ := stubBlockShot(t)
	for _, png64 := range []string{"", base64.StdEncoding.EncodeToString([]byte("not a png at all"))} {
		blockShotRPC = func(d wshrpc.CommandBlockShotData) (*wshrpc.CommandBlockShotRtnData, error) {
			return &wshrpc.CommandBlockShotRtnData{Png: png64, Width: 4, Height: 3}, nil
		}
		if err := blockShotCmd.RunE(blockShotCmd, nil); err == nil {
			t.Fatalf("a bad answer %q must be an error", png64)
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("no file for a bad picture: %v", entries)
	}
}

func TestBlockShotPassesTheWindowsErrorOn(t *testing.T) {
	stubBlockShot(t)
	blockShotRPC = func(d wshrpc.CommandBlockShotData) (*wshrpc.CommandBlockShotRtnData, error) {
		return nil, errors.New("the block is not drawn in its tab")
	}
	err := blockShotCmd.RunE(blockShotCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "not drawn") {
		t.Fatalf("the reason must reach the caller: %v", err)
	}
}
