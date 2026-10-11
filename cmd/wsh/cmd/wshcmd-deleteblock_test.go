// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"os"
	"strings"
	"testing"
)

func TestDeleteBlockAsksTheServerToRefuseARunningBlockUnlessForced(t *testing.T) {
	req := makeDeleteBlockRequest("b1", false)
	if req.BlockId != "b1" || !req.RefuseRunning {
		t.Fatalf("without --force the server must be told to refuse a running block: %+v", req)
	}
	forced := makeDeleteBlockRequest("b1", true)
	if forced.BlockId != "b1" || forced.RefuseRunning {
		t.Fatalf("--force deletes as before: %+v", forced)
	}
}

func TestDeleteBlockRunUsesTheRequestBuilderAndNothingElseDecides(t *testing.T) {
	src, err := os.ReadFile("wshcmd-deleteblock.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	run := text[strings.Index(text, "func deleteBlockRun"):]
	if !strings.Contains(run, "makeDeleteBlockRequest(fullORef.OID, deleteBlockForce)") {
		t.Fatalf("deleteBlockRun must send the request made by makeDeleteBlockRequest with the --force flag")
	}
	if strings.Contains(run, "&wshrpc.CommandDeleteBlockData{") || strings.Contains(run, "wshrpc.CommandDeleteBlockData{BlockId") {
		t.Fatalf("deleteBlockRun must not build its own request that skips the check")
	}
}
