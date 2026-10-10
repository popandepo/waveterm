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
)

const (
	TabMetaFlagColor        = "tab:flagcolor"
	TabMetaSymbol           = "tab:symbol"
	MaxTabSymbolTextBytes   = 32
	MaxTabSymbolTextRunes   = 8
	MaxTabSymbolIconNameLen = 40
	zeroWidthJoiner         = 0x200d
)

var tabStyleCmd = &cobra.Command{
	Use:   "style",
	Short: "Set the colour and symbol of a tab",
	Long: `Set the colour and the symbol shown on a tab in the tab bar.

The colour is a CSS colour name or a hex colour (#RRGGBB or #RRGGBBAA) and is stored in the existing
"tab:flagcolor" key. The symbol is a Font Awesome icon name (for example crown, star, rocket) or one or two
characters of text such as an emoji, and is stored in "tab:symbol".

By default the tab the command runs in is changed. Use --tab with a tab id, the first 8 characters of one,
"tab:<n>" (the n-th tab of the workspace) or a full tab:<uuid> reference to change another tab.`,
	Example: `  wsh tab style --color gold --symbol crown
  wsh tab style --symbol star
  wsh tab style --tab tab:2 --color "#ff8800"
  wsh tab style --clear`,
	Args:    cobra.NoArgs,
	RunE:    tabStyleRun,
	PreRunE: preRunSetupRpcClient,
}

var (
	tabStyleTab    string
	tabStyleColor  string
	tabStyleSymbol string
	tabStyleClear  bool
)

func init() {
	tabStyleCmd.Flags().StringVar(&tabStyleTab, "tab", "", "tab to change (default: the tab this command runs in)")
	tabStyleCmd.Flags().StringVar(&tabStyleColor, "color", "", "CSS colour name or hex colour")
	tabStyleCmd.Flags().StringVar(&tabStyleSymbol, "symbol", "", "Font Awesome icon name, or one or two characters (emoji)")
	tabStyleCmd.Flags().BoolVar(&tabStyleClear, "clear", false, "remove both the colour and the symbol")
	tabCommand.AddCommand(tabStyleCmd)
}

// buildTabStyleMeta returns the meta update for the given options. A nil value deletes the key.
func buildTabStyleMeta(color string, symbol string, clear bool) (map[string]any, error) {
	if hasHiddenOrDirectionalRunes(symbol) {
		return nil, fmt.Errorf("invalid symbol: control, bidirectional and invisible format characters are not allowed")
	}
	color = strings.TrimSpace(color)
	symbol = strings.TrimSpace(symbol)
	if clear {
		if color != "" || symbol != "" {
			return nil, fmt.Errorf("--clear cannot be combined with --color or --symbol")
		}
		return map[string]any{TabMetaFlagColor: nil, TabMetaSymbol: nil}, nil
	}
	if color == "" && symbol == "" {
		return nil, fmt.Errorf("nothing to set: give --color, --symbol or --clear")
	}
	meta := make(map[string]any)
	if color != "" {
		if err := validateColor(color); err != nil {
			return nil, err
		}
		meta[TabMetaFlagColor] = color
	}
	if symbol != "" {
		if err := validateTabSymbol(symbol); err != nil {
			return nil, err
		}
		meta[TabMetaSymbol] = symbol
	}
	return meta, nil
}

func isIconName(s string) bool {
	rest := s
	for _, prefix := range []string{"solid@", "regular@", "brands@", "custom@"} {
		if strings.HasPrefix(rest, prefix) {
			rest = rest[len(prefix):]
			break
		}
	}
	if rest == "" {
		return false
	}
	for _, r := range rest {
		if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '-' {
			return false
		}
	}
	return true
}

func hasHiddenOrDirectionalRunes(s string) bool {
	for _, r := range s {
		if r == zeroWidthJoiner {
			continue
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) {
			return true
		}
	}
	return false
}

func validateTabSymbol(symbol string) error {
	if hasHiddenOrDirectionalRunes(symbol) {
		return fmt.Errorf("invalid symbol: control, bidirectional and invisible format characters are not allowed")
	}
	if isIconName(symbol) {
		if len(symbol) > MaxTabSymbolIconNameLen {
			return fmt.Errorf("invalid symbol: icon names are limited to %d characters", MaxTabSymbolIconNameLen)
		}
		return nil
	}
	if len(symbol) > MaxTabSymbolTextBytes || len([]rune(symbol)) > MaxTabSymbolTextRunes || strings.ContainsAny(symbol, "<>&\"'`\\") {
		return fmt.Errorf("invalid symbol %q: use a Font Awesome icon name (lowercase, digits, dashes) or one or two characters such as an emoji (at most %d bytes)", symbol, MaxTabSymbolTextBytes)
	}
	return nil
}

func tabStyleRun(cmd *cobra.Command, args []string) (rtnErr error) {
	defer func() {
		sendActivity("tabstyle", rtnErr == nil)
	}()
	meta, err := buildTabStyleMeta(tabStyleColor, tabStyleSymbol, tabStyleClear)
	if err != nil {
		return err
	}
	ref := tabStyleTab
	if ref == "" {
		ref = "tab"
	}
	oref, err := resolveSimpleId(ref)
	if err != nil {
		return fmt.Errorf("resolving tab: %w", err)
	}
	if oref.OType != waveobj.OType_Tab {
		return fmt.Errorf("%q is a %s, not a tab", ref, oref.OType)
	}
	err = wshclient.SetMetaCommand(RpcClient, wshrpc.CommandSetMetaData{ORef: *oref, Meta: meta}, &wshrpc.RpcOpts{Timeout: 2000})
	if err != nil {
		return fmt.Errorf("setting tab style: %v", err)
	}
	WriteStdout("tab style set\n")
	return nil
}
