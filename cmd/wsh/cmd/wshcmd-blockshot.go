// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
	"github.com/wavetermdev/waveterm/pkg/wshutil"
)

// the one answer for a block the caller may not act on, whether it exists or not
const blockShotDenied = "that block is not available to this caller"

var blockShotCmd = &cobra.Command{
	Use:   "blockshot [-b block]",
	Short: "save a picture of one block (any view, own workspace), shown or not, to a new PNG file",
	Long: `Saves a PNG of ONE block, cut out of the loaded page of its tab, and prints its path. The tab does not have to be shown: tabs that are not
shown are loaded and parked off screen, and this takes the picture from there without showing, moving or focusing anything. Only blocks in the
caller's own workspace. The file goes into the same folder as 'wsh web shot' (AppData\Local\Wave\web-shots of your profile; the 50 newest are kept)
with a generated name; the caller does not choose a folder or a name, and no file is ever written over.
A block behind another member of a stack, or in a tab whose page is not loaded yet, gives an error, not a blank file.`,
	Args: cobra.NoArgs,
	RunE: blockShotRun,
}

func init() {
	rootCmd.AddCommand(blockShotCmd)
}

// seam for tests
var blockShotRPC = func(data wshrpc.CommandBlockShotData) (*wshrpc.CommandBlockShotRtnData, error) {
	return wshclient.BlockShotCommand(RpcClient, data, &wshrpc.RpcOpts{Route: wshutil.ElectronRoute, Timeout: 15000})
}

func blockShotRun(cmd *cobra.Command, args []string) error {
	ref := blockArg
	if ref == "" {
		ref = "this"
	}
	blockId, err := resolveBlockRef(ref)
	if err != nil {
		if blockArg != "" {
			return errors.New(blockShotDenied)
		}
		return fmt.Errorf("resolving blockid: %w", err)
	}
	if err := webNavAuthorize(blockId); err != nil {
		return errors.New(blockShotDenied)
	}
	info, err := webNavBlockInfo(blockId)
	if err != nil {
		return fmt.Errorf("getting block info: %w", err)
	}
	if info == nil || info.Block == nil || info.TabId == "" || info.WorkspaceId == "" {
		return fmt.Errorf("block %s has no tab or workspace", blockId)
	}
	res, err := blockShotRPC(wshrpc.CommandBlockShotData{WorkspaceId: info.WorkspaceId, TabId: info.TabId, BlockId: blockId})
	if err != nil {
		return err
	}
	data, err := decodeWebShot(&wshrpc.CommandWebShotRtnData{Png: res.Png, Width: res.Width, Height: res.Height})
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
	if err := writeNewShotFile(filepath.Join(dir, name), data); err != nil {
		return fmt.Errorf("writing the picture: %w", err)
	}
	WriteStdout("%s (%dx%d, %d bytes)\n", filepath.Join(dir, name), res.Width, res.Height, len(data))
	if err := pruneWebShots(dir, name); err != nil {
		WriteStderr("removing older pictures: %v\n", err)
	}
	return nil
}
