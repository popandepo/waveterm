// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
	"github.com/wavetermdev/waveterm/pkg/wshutil"
)

// The largest picture that is written, in bytes of PNG. emain applies the same cap before it sends.
const maxWebShotBytes = 4 * 1024 * 1024

var pngSignature = []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}

var webShotCmd = &cobra.Command{
	Use:   "shot [-b block]",
	Short: "save a picture of one web tile (own workspace, and only a tile with its own web:partition) to a new PNG file",
	Long: `Saves a PNG of the web contents of ONE tile, not of the window, and prints its path. Reads only: nothing is navigated, clicked or
focused. The file always goes into the tool's own folder (web-shots under the local application data folder, Wave\web-shots), with a new
name made of the block id, the UTC time and a random part; the caller does not choose a folder or a name, and no file is ever written over.
The tile must be shown (a tile in a tab that is not shown gives an empty picture, which is an error, not a blank file).

A web tile that has no web:partition of its own uses the default sign-in store of Wave, which every web tile shares and which keeps
logins across restarts: a picture of it is refused, as its text is, until the owner can allow a tile by name.`,
	Args: cobra.NoArgs,
	RunE: webShotRun,
}

func init() {
	webCmd.AddCommand(webShotCmd)
}

// seams for tests
var (
	webShotRPC = func(data wshrpc.CommandWebShotData) (*wshrpc.CommandWebShotRtnData, error) {
		return wshclient.WebShotCommand(RpcClient, data, &wshrpc.RpcOpts{Route: wshutil.ElectronRoute, Timeout: 15000})
	}
	// the folder the pictures go to: the tool's own, never one the caller names
	webShotDir = func() (string, error) {
		base, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(base, "Wave", "web-shots"), nil
	}
	webShotWriteFile = func(f *os.File, data []byte) error {
		_, err := f.Write(data)
		return err
	}
)

// webShotName is <blockid>-<utc time>-<random>.png: unique without looking at the folder, and nothing in it comes from the caller but the block id,
// which is a checked full id by then.
func webShotName(blockId string, now time.Time) (string, error) {
	var r [4]byte
	if _, err := rand.Read(r[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s-%s.png", blockId, now.UTC().Format("20060102T150405Z"), hex.EncodeToString(r[:])), nil
}

// decodeWebShot turns the answer into the bytes of a PNG, or refuses it: not base64, too large, or not a complete PNG (the whole picture is decoded).
func decodeWebShot(res *wshrpc.CommandWebShotRtnData) ([]byte, error) {
	if res == nil || res.Png == "" {
		return nil, fmt.Errorf("no picture from the window")
	}
	if len(res.Png) > maxWebShotBytes*4/3+16 {
		return nil, fmt.Errorf("the picture is larger than %d bytes", maxWebShotBytes)
	}
	data, err := base64.StdEncoding.DecodeString(res.Png)
	if err != nil {
		return nil, fmt.Errorf("the picture from the window is not valid base64")
	}
	if len(data) > maxWebShotBytes {
		return nil, fmt.Errorf("the picture is larger than %d bytes", maxWebShotBytes)
	}
	if !bytes.HasPrefix(data, pngSignature) {
		return nil, fmt.Errorf("the picture from the window is not a PNG")
	}
	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		return nil, fmt.Errorf("the picture from the window is not a complete PNG: %w", err)
	}
	return data, nil
}

// writeNewShotFile creates path, which must not exist, and writes the picture. A write that fails removes the file again (on Windows through its
// own handle) and says so if that removal failed.
func writeNewShotFile(path string, data []byte) error {
	f, err := createShotFile(path)
	if err != nil {
		return err
	}
	if err := webShotWriteFile(f, data); err != nil {
		if derr := discardShotFile(f, path); derr != nil {
			return fmt.Errorf("%w (and the partial file %s could not be removed: %v)", err, path, derr)
		}
		return err
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing %s: %w (the file may be incomplete)", path, err)
	}
	return nil
}

func webShotRun(cmd *cobra.Command, args []string) error {
	ref := blockArg
	if ref == "" {
		ref = "this"
	}
	blockId, err := resolveBlockRef(ref)
	if err != nil {
		if blockArg != "" {
			return errors.New(webNavDenied)
		}
		return fmt.Errorf("resolving blockid: %w", err)
	}
	if err := webNavAuthorize(blockId); err != nil {
		return errors.New(webNavDenied)
	}
	info, err := webNavBlockInfo(blockId)
	if err != nil {
		return fmt.Errorf("getting block info: %w", err)
	}
	nav, err := makeWebNavRequest(info, blockId, wshrpc.WebNavAction_Url)
	if err != nil {
		return err
	}
	res, err := webShotRPC(wshrpc.CommandWebShotData{WorkspaceId: nav.WorkspaceId, TabId: nav.TabId, BlockId: blockId})
	if err != nil {
		if strings.Contains(err.Error(), webNavDenied) {
			return errors.New(webNavDenied)
		}
		return err
	}
	data, err := decodeWebShot(res)
	if err != nil {
		return err
	}
	dir, err := webShotDir()
	if err != nil {
		return fmt.Errorf("the folder for pictures: %w", err)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("the folder for pictures: %w", err)
	}
	name, err := webShotName(blockId, time.Now())
	if err != nil {
		return err
	}
	target := filepath.Join(dir, name)
	if err := writeNewShotFile(target, data); err != nil {
		return fmt.Errorf("writing the picture: %w", err)
	}
	WriteStdout("%s (%dx%d, %d bytes)\n", target, res.Width, res.Height, len(data))
	return nil
}
