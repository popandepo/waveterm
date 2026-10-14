// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
	"github.com/wavetermdev/waveterm/pkg/wshutil"
)

// The largest picture that is written, in bytes of PNG. emain applies the same cap before it sends.
const maxWebShotBytes = 4 * 1024 * 1024

var pngSignature = []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}

var webShotOut string

var webShotCmd = &cobra.Command{
	Use:   "shot --out file.png [-b block]",
	Short: "save a picture of one web tile (own workspace, and only a tile with its own web:partition) to a new PNG file",
	Long: `Saves a PNG of the web contents of ONE tile, not of the window. Reads only: nothing is navigated, clicked or focused.
The file must not exist yet (it is never overwritten), its folder must exist, its name must end in .png, and neither the folder nor
the path may be inside a .claude or .claude-work folder. The tile must be shown (a tile in a tab that is not shown gives an empty
picture, which is an error, not a blank file).

A web tile that has no web:partition of its own uses the default sign-in store of Wave, which every web tile shares and which keeps
logins across restarts: a picture of it is refused, as its text is, until the owner can allow a tile by name.`,
	Args: cobra.NoArgs,
	RunE: webShotRun,
}

func init() {
	webShotCmd.Flags().StringVar(&webShotOut, "out", "", "the new .png file to write (required; must not exist)")
	webCmd.AddCommand(webShotCmd)
}

// seam for tests
var webShotRPC = func(data wshrpc.CommandWebShotData) (*wshrpc.CommandWebShotRtnData, error) {
	return wshclient.WebShotCommand(RpcClient, data, &wshrpc.RpcOpts{Route: wshutil.ElectronRoute, Timeout: 15000})
}

// Folders that this tool never writes into: the profile folders of the other Claude installations.
func isOffLimitsSegment(seg string) bool {
	l := strings.ToLower(seg)
	return l == ".claude" || l == ".claude-work"
}

func hasOffLimitsSegment(path string) bool {
	for _, seg := range strings.FieldsFunc(path, func(r rune) bool { return r == '\\' || r == '/' }) {
		if isOffLimitsSegment(seg) {
			return true
		}
	}
	return false
}

// checkWebShotPath decides where the picture may go, before any request is made: an absolute spelling that is checked as written AND after its folder
// is resolved (a junction or a link cannot lead into an off-limits folder), a .png name, an existing folder, and a file that does not exist.
func checkWebShotPath(out string) (string, error) {
	if strings.TrimSpace(out) == "" {
		return "", fmt.Errorf("--out is required: the new .png file to write")
	}
	if strings.ContainsAny(out, "\x00\r\n") {
		return "", fmt.Errorf("--out cannot contain control characters")
	}
	abs, err := filepath.Abs(out)
	if err != nil {
		return "", fmt.Errorf("--out: %w", err)
	}
	if !strings.EqualFold(filepath.Ext(abs), ".png") {
		return "", fmt.Errorf("--out must end in .png")
	}
	if hasOffLimitsSegment(abs) {
		return "", fmt.Errorf("--out cannot be inside a .claude or .claude-work folder")
	}
	dir := filepath.Dir(abs)
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		return "", fmt.Errorf("the folder of --out does not exist: %s", dir)
	}
	resolved, err := finalFolderPath(dir)
	if err != nil {
		return "", fmt.Errorf("the folder of --out cannot be resolved: %w", err)
	}
	if hasOffLimitsSegment(resolved) {
		return "", fmt.Errorf("--out cannot be inside a .claude or .claude-work folder")
	}
	final := filepath.Join(resolved, filepath.Base(abs))
	if _, err := os.Lstat(final); err == nil {
		return "", fmt.Errorf("%s already exists (a picture is never written over a file)", final)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("--out: %w", err)
	}
	return final, nil
}

// decodeWebShot turns the answer into the bytes of a PNG, or refuses it: not base64, too large, or not a PNG.
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
	return data, nil
}

// writeNewFile creates the file and fails if it exists (a race between the check and the write does not overwrite anything). The folder was
// checked before the request and can be swapped while the window answers, so the new file must turn out to be in that same checked folder before
// anything is written (createNewFileIn).
func writeNewFile(path string, data []byte) error {
	f, err := createNewFileIn(path, filepath.Dir(path))
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func webShotRun(cmd *cobra.Command, args []string) error {
	target, err := checkWebShotPath(webShotOut)
	if err != nil {
		return err
	}
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
	if err := writeNewFile(target, data); err != nil {
		return fmt.Errorf("writing the picture: %w", err)
	}
	WriteStdout("%s (%dx%d, %d bytes)\n", target, res.Width, res.Height, len(data))
	return nil
}
