// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package blockcontroller

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/wavetermdev/waveterm/pkg/util/shellutil"
	"github.com/wavetermdev/waveterm/pkg/wavebase"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wstore"
)

// The environment a new shell gets (the swap token) carries the block's history file for a local PowerShell, and nothing for other shells
// or remote connections. A real store is used because makeSwapToken looks up the tab of the block.
func TestTheShellEnvironmentCarriesTheBlocksHistoryFileForPowerShellOnly(t *testing.T) {
	dataDir := t.TempDir()
	old := wavebase.DataHome_VarCache
	wavebase.DataHome_VarCache = dataDir
	t.Cleanup(func() { wavebase.DataHome_VarCache = old })
	if err := os.MkdirAll(filepath.Join(dataDir, wavebase.WaveDBDir), 0700); err != nil {
		t.Fatal(err)
	}
	if err := wstore.InitWStore(); err != nil {
		t.Fatalf("init wstore: %v", err)
	}
	t.Cleanup(func() { _ = wstore.CloseDB() })
	ctx := context.Background()
	block := &waveobj.Block{OID: uuid.NewString(), Meta: waveobj.MetaMapType{"view": "term"}}
	if err := wstore.DBInsert(ctx, block); err != nil {
		t.Fatal(err)
	}
	want := shellutil.BlockHistoryFile(dataDir, block.OID)

	pwsh := makeSwapToken(ctx, ctx, block.OID, block.Meta, "", shellutil.ShellType_pwsh)
	if pwsh.Env[shellutil.PwshHistFileEnvName] != want {
		t.Fatalf("a local PowerShell must get its block's history file %q, got %q", want, pwsh.Env[shellutil.PwshHistFileEnvName])
	}
	bash := makeSwapToken(ctx, ctx, block.OID, block.Meta, "", "bash")
	if v, ok := bash.Env[shellutil.PwshHistFileEnvName]; ok {
		t.Fatalf("bash must not get it, got %q", v)
	}
	remote := makeSwapToken(ctx, ctx, block.OID, block.Meta, "user@server", shellutil.ShellType_pwsh)
	if v, ok := remote.Env[shellutil.PwshHistFileEnvName]; ok {
		t.Fatalf("a remote connection must not get it, got %q", v)
	}
}
