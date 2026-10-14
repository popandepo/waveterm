// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// packswaptoken prints a swap token packed by Wave's own code (TokenSwapEntry.PackForClient), shaped as the block controller makes it for a local
// shell. Test-ResumableSession.ps1 uses it so that the starter is tested against the real format, not a hand-built one. Test use only: the token is
// never registered with a server.
package main

import (
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/wavetermdev/waveterm/pkg/util/shellutil"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
)

func main() {
	if len(os.Args) != 2 || os.Args[1] == "" {
		fmt.Fprintln(os.Stderr, "usage: packswaptoken <socket name>")
		os.Exit(2)
	}
	entry := &shellutil.TokenSwapEntry{
		Token: uuid.New().String(),
		RpcContext: &wshrpc.RpcContext{
			ProcRoute: true,
			SockName:  os.Args[1],
			BlockId:   uuid.New().String(),
		},
	}
	packed, err := entry.PackForClient()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Print(packed)
}
