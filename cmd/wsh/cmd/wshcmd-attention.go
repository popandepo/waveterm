// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/attention"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
)

var attentionReason string

var attentionCmd = &cobra.Command{
	Use:   "attention",
	Short: "mark a block as needing the owner (info, need, urgent) or clear the mark",
	Long: `Marks a block as needing attention. The mark climbs from the block to its stack tab, its tile and its top tab.
  info    an icon, no motion
  need    an orange dot (needs a reason, shown on hover)
  urgent  a flashing orange outline and dot (needs a reason, shown on hover)
The orange levels clear when the owner focuses the tile, or when a session runs "wsh attention clear".`,
}

func attentionLevelCmd(level string) *cobra.Command {
	return &cobra.Command{
		Use:     level + " --reason \"why\" [-b block]",
		Short:   "mark the block with the level " + level,
		Args:    cobra.NoArgs,
		RunE:    func(cmd *cobra.Command, args []string) error { return attentionSetRun(level) },
		PreRunE: preRunSetupRpcClient,
	}
}

var attentionClearCmd = &cobra.Command{
	Use:     "clear [-b block]",
	Short:   "clear the attention mark of the block",
	Args:    cobra.NoArgs,
	RunE:    attentionClearRun,
	PreRunE: preRunSetupRpcClient,
}

func init() {
	for _, level := range []string{attention.LevelInfo, attention.LevelNeed, attention.LevelUrgent} {
		c := attentionLevelCmd(level)
		c.Flags().StringVar(&attentionReason, "reason", "", "why the owner is needed (required for need and urgent, at most 120 characters)")
		attentionCmd.AddCommand(c)
	}
	attentionCmd.AddCommand(attentionClearCmd)
	rootCmd.AddCommand(attentionCmd)
}

// attentionMeta is the meta that marks a block; the check is the one the window applies when it draws the mark.
func attentionMeta(level string, reason string) (map[string]any, error) {
	cleaned, err := attention.Check(level, reason)
	if err != nil {
		return nil, err
	}
	meta := map[string]any{attention.MetaKeyLevel: level, attention.MetaKeyReason: nil}
	if cleaned != "" {
		meta[attention.MetaKeyReason] = cleaned
	}
	return meta, nil
}

func attentionClearMeta() map[string]any {
	return map[string]any{attention.MetaKeyLevel: nil, attention.MetaKeyReason: nil}
}

func attentionSend(oref waveobj.ORef, meta map[string]any) error {
	return wshclient.SetMetaCommand(RpcClient, wshrpc.CommandSetMetaData{ORef: oref, Meta: meta}, &wshrpc.RpcOpts{Timeout: 2000})
}

func attentionSetRun(level string) error {
	meta, err := attentionMeta(level, attentionReason)
	if err != nil {
		return err
	}
	oref, err := resolveBlockArg()
	if err != nil {
		return err
	}
	if oref.OType != waveobj.OType_Block {
		return fmt.Errorf("%s is a %s, attention marks a block", oref, oref.OType)
	}
	if err := attentionSend(*oref, meta); err != nil {
		return fmt.Errorf("setting the attention mark: %w", err)
	}
	WriteStdout("attention %s set\n", level)
	return nil
}

func attentionClearRun(cmd *cobra.Command, args []string) error {
	oref, err := resolveBlockArg()
	if err != nil {
		return err
	}
	if oref.OType != waveobj.OType_Block {
		return fmt.Errorf("%s is a %s, attention marks a block", oref, oref.OType)
	}
	if err := attentionSend(*oref, attentionClearMeta()); err != nil {
		return fmt.Errorf("clearing the attention mark: %w", err)
	}
	WriteStdout("attention cleared\n")
	return nil
}
