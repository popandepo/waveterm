// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package shellutil

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/wavetermdev/waveterm/pkg/wavebase"
)

const (
	HistoryDirName      = "history"
	PwshHistFileEnvName = "WAVETERM_PWSH_HISTFILE"
)

var blockIdRegexp = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// BlockHistoryFile is the command history file of one block: <data dir>\history\<block id>.txt. Only a block id in the form of a
// uuid gives a path; anything else gives "" so that a crafted id can never point outside the folder.
func BlockHistoryFile(dataDir string, blockId string) string {
	if !blockIdRegexp.MatchString(blockId) {
		return ""
	}
	return filepath.Join(dataDir, HistoryDirName, blockId+".txt")
}

// PerBlockHistoryEnv is what a PowerShell block gets so that its command history is its own and not the one file that every
// PowerShell on the machine shares: the path of its history file, and the folder made. Only for a local PowerShell (the data
// folder is on this machine), and only when the setting is not switched off (it is on unless it is explicitly false).
func PerBlockHistoryEnv(enabled *bool, shellType string, remoteName string, blockId string) map[string]string {
	if enabled != nil && !*enabled {
		return nil
	}
	if shellType != ShellType_pwsh {
		return nil
	}
	if remoteName != "" && remoteName != "local" && !isLocalVariant(remoteName) {
		return nil
	}
	file := BlockHistoryFile(wavebase.GetWaveDataDir(), blockId)
	if file == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		return nil
	}
	return map[string]string{PwshHistFileEnvName: file}
}

func isLocalVariant(name string) bool {
	return len(name) > len("local:") && name[:len("local:")] == "local:"
}

// RemoveBlockHistory deletes the history file of a block that is gone. One named file, nothing else; a missing file is not an error.
func RemoveBlockHistory(dataDir string, blockId string) error {
	file := BlockHistoryFile(dataDir, blockId)
	if file == "" {
		return nil
	}
	if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing %s: %w", file, err)
	}
	return nil
}
