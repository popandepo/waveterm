// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"strings"
	"unicode"

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

Control characters in the text (escape sequences a program drew, bells, backspaces) are shown as \x1b style escapes
instead of being written to your terminal, and bidirectional or invisible format characters as \u202e style escapes.
Output is limited to --max-bytes; when lines are dropped the first line says so.

The block's terminal view must be loaded, which it is for any tab that has been shown since the app started (up to
10 tabs are kept loaded). An error says so when it is not.`,
	Example: `  wsh screen -b 3f2a9c10
  wsh screen -b 3 --last 20`,
	Args:    cobra.NoArgs,
	RunE:    screenRun,
	PreRunE: preRunSetupRpcClient,
}

const DefaultScreenMaxBytes = 200000

var (
	screenLast     int
	screenMaxBytes int
)

func init() {
	screenCmd.Flags().IntVar(&screenLast, "last", 0, "only the last N lines (0 = everything)")
	screenCmd.Flags().IntVar(&screenMaxBytes, "max-bytes", DefaultScreenMaxBytes, "upper limit for the output; the oldest lines are dropped and the cut is reported")
	rootCmd.AddCommand(screenCmd)
}

// sanitizeScreenLine makes one line safe to print into the caller's terminal.
func sanitizeScreenLine(line string) string {
	var sb strings.Builder
	for _, r := range line {
		switch {
		case r == '\t':
			sb.WriteRune(r)
		case r < 0x100 && (unicode.IsControl(r)):
			fmt.Fprintf(&sb, "\\x%02x", r)
		case unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r):
			fmt.Fprintf(&sb, "\\u%04x", r)
		default:
			sb.WriteRune(r)
		}
	}
	return sb.String()
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

// renderScreen trims, sanitizes and bounds the screen text. When the text is longer than maxBytes the oldest lines
// are dropped and a first line says how many.
func renderScreen(lines []string, last int, maxBytes int) string {
	lines = trimScreenLines(lines, last)
	safe := make([]string, len(lines))
	for i, l := range lines {
		safe[i] = sanitizeScreenLine(l)
	}
	total := len(safe)
	size := 0
	start := len(safe)
	for start > 0 && (maxBytes <= 0 || size+len(safe[start-1])+1 <= maxBytes) {
		start--
		size += len(safe[start]) + 1
	}
	if start == len(safe) && total > 0 {
		start = len(safe) - 1
	}
	out := safe[start:]
	if start > 0 {
		out = append([]string{fmt.Sprintf("[wsh screen: output limited to %d bytes, showing the last %d of %d lines]", maxBytes, len(out), total)}, out...)
	}
	return strings.Join(out, "\n")
}

type screenFetcher func(blockId string) ([]string, error)

// readBlockScreen asks for the lines of exactly the given block.
func readBlockScreen(blockId string, fetch screenFetcher, last int, maxBytes int) (string, error) {
	lines, err := fetch(blockId)
	if err != nil {
		return "", err
	}
	return renderScreen(lines, last, maxBytes), nil
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
	fetch := func(blockId string) ([]string, error) {
		result, err := wshclient.TermGetScrollbackLinesCommand(RpcClient, wshrpc.CommandTermGetScrollbackLinesData{}, &wshrpc.RpcOpts{
			Route:   wshutil.MakeFeBlockRouteId(blockId),
			Timeout: 5000,
		})
		if err != nil {
			return nil, fmt.Errorf("could not read block %s (its terminal view must be loaded; show its tab once): %w", blockId, err)
		}
		return result.Lines, nil
	}
	text, err := readBlockScreen(fullORef.OID, fetch, screenLast, screenMaxBytes)
	if err != nil {
		return err
	}
	if text != "" {
		WriteStdout("%s\n", text)
	}
	return nil
}
