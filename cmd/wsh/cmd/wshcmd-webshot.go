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
	"regexp"
	"sort"
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
focused. The file always goes into the tool's own folder (AppData\Local\Wave\web-shots of your profile; the 50 newest are kept), with a new
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
	webShotDir       = defaultWebShotDir
	webShotWriteFile = func(f *os.File, data []byte) error {
		_, err := f.Write(data)
		return err
	}
	webShotSyncFile = func(f *os.File) error { return f.Sync() }
)

// How many pictures the folder keeps; older ones named like ours are removed after each new one.
const webShotKeep = 50

var webShotNameRe = regexp.MustCompile(`^[0-9a-fA-F-]{36}-\d{8}T\d{6}Z-[0-9a-f]{8}\.png$`)

// The largest picture by its declared size: a small PNG can claim huge dimensions, and decoding allocates for what it claims.
const (
	maxWebShotSide   = 8192
	maxWebShotPixels = 16 * 1024 * 1024
)

// pruneWebShots keeps the newest webShotKeep pictures in dir and removes older files named like ours; nothing else in the folder is touched. The
// order is the UTC time in the name (then the whole name), not the file times, which anything can change. keep (the picture just saved) is never
// removed, whatever the other names claim. Best effort: two shots at once can each count before the other writes, and leave one more than the limit.
func pruneWebShots(dir string, keep string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var ours []string
	for _, e := range entries {
		if e.Type().IsRegular() && webShotNameRe.MatchString(e.Name()) && e.Name() != keep {
			ours = append(ours, e.Name())
		}
	}
	// the picture just saved is one of the webShotKeep
	if len(ours) <= webShotKeep-1 {
		return nil
	}
	sort.Slice(ours, func(i, j int) bool {
		ti, tj := ours[i][37:53], ours[j][37:53]
		if ti != tj {
			return ti < tj
		}
		return ours[i] < ours[j]
	})
	var firstErr error
	for _, name := range ours[:len(ours)-(webShotKeep-1)] {
		if err := os.Remove(filepath.Join(dir, name)); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

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
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("the picture from the window is not a PNG: %w", err)
	}
	if cfg.Width > maxWebShotSide || cfg.Height > maxWebShotSide || cfg.Width*cfg.Height > maxWebShotPixels {
		return nil, fmt.Errorf("the picture claims %dx%d pixels, more than %d per side or %d in all", cfg.Width, cfg.Height, maxWebShotSide, maxWebShotPixels)
	}
	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		return nil, fmt.Errorf("the picture from the window is not a complete PNG: %w", err)
	}
	return data, nil
}

// writeNewShotFile creates path, which must not exist, writes the picture and flushes it. A write or flush that fails removes the file again (on
// Windows through its own handle: the file is made delete-pending and only kept once the whole picture is on disk) and says so if that failed.
func writeNewShotFile(path string, data []byte) error {
	f, err := createShotFile(path)
	if err != nil {
		return err
	}
	err = webShotWriteFile(f, data)
	if err == nil {
		err = webShotSyncFile(f)
	}
	if err != nil {
		if derr := discardShotFile(f, path); derr != nil {
			return fmt.Errorf("%w (and the partial file %s could not be removed: %v)", err, path, derr)
		}
		return err
	}
	return keepShotFile(f, path)
}

// ensureShotDir makes the shot folder and its parent (Wave) one at a time, each refused unless it is a plain folder, so neither an existing link at
// Wave nor one at web-shots can lead the folder anywhere; nothing is made under a component that fails the check. The folders above Wave (the
// profile's AppData\Local, or the home's .cache) are trusted: they belong to the user, and a link there is the user's own doing.
func ensureShotDir(dir string) error {
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(filepath.Dir(parent), 0700); err != nil {
		return fmt.Errorf("the folder for pictures: %w", err)
	}
	for _, p := range []string{parent, dir} {
		if _, err := os.Lstat(p); errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(p, 0700); err != nil && !errors.Is(err, os.ErrExist) {
				return fmt.Errorf("the folder for pictures: %w", err)
			}
		}
		if err := checkShotDir(p); err != nil {
			return err
		}
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
	if err := ensureShotDir(dir); err != nil {
		return err
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
	if err := pruneWebShots(dir, name); err != nil {
		WriteStderr("removing older pictures: %v\n", err)
	}
	return nil
}
