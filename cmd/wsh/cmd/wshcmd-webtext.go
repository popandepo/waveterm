// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
	"github.com/wavetermdev/waveterm/pkg/wshutil"
)

// The most one call returns, in bytes. emain applies the same cap; --max-bytes can only lower it.
const maxWebTextBytes = 262144

var (
	webTextSelector string
	webTextMax      int
)

var webTextCmd = &cobra.Command{
	Use:   "text [--selector css] [--max-bytes n] [-b block]",
	Short: "print the visible text of a web tile (own workspace, and only a tile with its own web:partition)",
	Long: `Prints the visible text (innerText) of the page, or of the first element that matches --selector, cut at --max-bytes (at most 262144).
Reads only: nothing is navigated, clicked or focused.

A web tile that has no web:partition of its own uses the default sign-in store of Wave, which every web tile shares and which keeps logins
across restarts. The text of such a tile is refused until the owner can allow a tile by name. Same limits as the other web commands: a
tile of the caller's own workspace only. Control characters and invisible format characters in the page text are printed as <U+XXXX>.`,
	Args: cobra.NoArgs,
	RunE: webTextRun,
}

func init() {
	webTextCmd.Flags().StringVar(&webTextSelector, "selector", "", "css selector: the text of the first match instead of the whole page")
	webTextCmd.Flags().IntVar(&webTextMax, "max-bytes", maxWebTextBytes, "cut the text after this many bytes of UTF-8 (at most 262144)")
	webCmd.AddCommand(webTextCmd)
}

// seam for tests
var webTextRPC = func(data wshrpc.CommandWebTextData) (*wshrpc.CommandWebTextRtnData, error) {
	return wshclient.WebTextCommand(RpcClient, data, &wshrpc.RpcOpts{Route: wshutil.ElectronRoute, Timeout: 10000})
}

func checkWebTextOptions(selector string, maxBytes int) error {
	if maxBytes < 1 || maxBytes > maxWebTextBytes {
		return fmt.Errorf("--max-bytes must be between 1 and %d", maxWebTextBytes)
	}
	if len(selector) > 512 {
		return fmt.Errorf("--selector is at most 512 characters")
	}
	for _, r := range selector {
		if unicode.IsControl(r) {
			return fmt.Errorf("--selector cannot contain control characters")
		}
	}
	return nil
}

// safeTextBlock is safeText for a page's text: line breaks and tabs stay, every other control, format or separator character is shown as <U+XXXX>
// (a page controls all of it, and the text goes to a terminal), and a carriage return is one of those.
func safeTextBlock(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) || r == unicode.ReplacementChar:
			b.WriteString(fmt.Sprintf("<U+%04X>", r))
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func webTextRun(cmd *cobra.Command, args []string) error {
	if err := checkWebTextOptions(webTextSelector, webTextMax); err != nil {
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
	// who may act on which tile is asked first, as for the navigation commands
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
	res, err := webTextRPC(wshrpc.CommandWebTextData{WorkspaceId: nav.WorkspaceId, TabId: nav.TabId, BlockId: blockId, Selector: webTextSelector, MaxBytes: webTextMax})
	if err != nil {
		if strings.Contains(err.Error(), webNavDenied) {
			return errors.New(webNavDenied)
		}
		// the window's error can carry text the page wrote (a throwing innerText getter)
		return errors.New(safeText(err.Error(), maxPrintedErrorRunes))
	}
	if res == nil {
		return fmt.Errorf("no answer from the window")
	}
	// the newline after the text counts toward --max-bytes
	out, cut := webTextOutput(res.Text, webTextMax-1)
	WriteStdout("%s\n", out)
	if cut || res.Truncated {
		WriteStderr("the text was cut (at most %d bytes)\n", webTextMax)
	}
	return nil
}

const maxPrintedErrorRunes = 400

// webTextOutput is the printable text cut to maxBytes. The cut comes after escaping, which can make text up to eight times longer, and never splits a
// character or a <U+XXXX>.
func webTextOutput(text string, maxBytes int) (string, bool) {
	var b strings.Builder
	for _, r := range text {
		piece := safeTextBlock(string(r))
		if b.Len()+len(piece) > maxBytes {
			return b.String(), true
		}
		b.WriteString(piece)
	}
	return b.String(), false
}
