// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package blockcontroller

import (
	"reflect"
	"testing"

	"github.com/wavetermdev/waveterm/pkg/waveobj"
)

// A cmd block with cmd:shell false hands the program and its arguments to a local connection as a list; with cmd:shell true (the default)
// there is no such list and the command line goes to the shell as before.
func TestCmdBlockWithoutShellHasADirectArgumentList(t *testing.T) {
	meta := waveobj.MetaMapType{
		waveobj.MetaKey_Cmd:      "pwsh",
		waveobj.MetaKey_CmdArgs:  []any{"-NoLogo", "-File", `C:\a b\start.ps1`, "-Prompt", `a; b "c"`},
		waveobj.MetaKey_CmdShell: false,
	}
	cmdStr, opts, err := createCmdStrAndOpts("block", meta, "local")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"pwsh", "-NoLogo", "-File", `C:\a b\start.ps1`, "-Prompt", `a; b "c"`}
	if !reflect.DeepEqual(opts.Direct, want) {
		t.Fatalf("direct list %q, want %q", opts.Direct, want)
	}
	if cmdStr == "pwsh" {
		t.Fatalf("the quoted command line for remote connections lost its arguments: %q", cmdStr)
	}

	meta[waveobj.MetaKey_CmdShell] = true
	meta[waveobj.MetaKey_Cmd] = "pwsh -NoLogo -c exit"
	_, opts, err = createCmdStrAndOpts("block", meta, "local")
	if err != nil {
		t.Fatal(err)
	}
	if len(opts.Direct) != 0 {
		t.Fatalf("a block that uses the shell got a direct list: %q", opts.Direct)
	}
}
