// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
	"github.com/wavetermdev/waveterm/pkg/wshutil"
)

var screenCmd = &cobra.Command{
	Use:   "screen [-b block]",
	Short: "Print the text on the screen of one terminal block",
	Long: `Print the text currently on the screen (and in the scrollback) of ONE terminal block, by block reference, without
focusing it and whether or not its tab is the active one. Blank lines below the last line of text are removed, which
is what makes this different from 'wsh termscrollback --start 0 --end N': that command counts lines from the bottom
of the whole terminal grid, so on a mostly empty screen its first lines are blank.

The block's terminal view must be loaded, which it is for any tab that has been shown since the app started (up to
10 tabs are kept loaded). An error says so when it is not.`,
	Example: `  wsh screen -b 3f2a9c10
  wsh screen -b 3 --last 20`,
	Args:    cobra.NoArgs,
	RunE:    screenRun,
	PreRunE: preRunSetupRpcClient,
}

var screenLast int

func init() {
	screenCmd.Flags().IntVar(&screenLast, "last", 0, "only the last N lines (0 = everything)")
	rootCmd.AddCommand(screenCmd)
}

// trimScreenLines removes trailing blank lines and, when last > 0, keeps only the last `last` lines.
func trimScreenLines(lines []string, last int) []string {
	end := len(lines)
	for end > 0 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	lines = append([]string{}, lines[:end]...)
	if last > 0 && len(lines) > last {
		lines = lines[len(lines)-last:]
	}
	return lines
}

func screenRun(cmd *cobra.Command, args []string) (rtnErr error) {
	defer func() {
		sendActivity("screen", rtnErr == nil)
	}()
	fullORef, err := resolveBlockArg()
	if err != nil {
		return err
	}
	metaData, err := wshclient.GetMetaCommand(RpcClient, wshrpc.CommandGetMetaData{ORef: *fullORef}, &wshrpc.RpcOpts{Timeout: 2000})
	if err != nil {
		return fmt.Errorf("error getting block metadata: %w", err)
	}
	viewType, _ := metaData[waveobj.MetaKey_View].(string)
	if viewType != "term" {
		return fmt.Errorf("block %s is not a terminal block (view type: %s)", fullORef.OID, viewType)
	}
	result, err := wshclient.TermGetScrollbackLinesCommand(RpcClient, wshrpc.CommandTermGetScrollbackLinesData{}, &wshrpc.RpcOpts{
		Route:   wshutil.MakeFeBlockRouteId(fullORef.OID),
		Timeout: 5000,
	})
	if err != nil {
		return fmt.Errorf("could not read block %s (its terminal view must be loaded; show its tab once): %w", fullORef.OID, err)
	}
	lines := trimScreenLines(result.Lines, screenLast)
	if len(lines) > 0 {
		WriteStdout("%s\n", strings.Join(lines, "\n"))
	}
	return nil
}
