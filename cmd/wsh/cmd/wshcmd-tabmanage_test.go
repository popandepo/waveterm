// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"testing"

	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
)

func makeWorkspaceInfo(wsId string, tabIds ...string) wshrpc.WorkspaceInfoData {
	return wshrpc.WorkspaceInfoData{WorkspaceData: &waveobj.Workspace{OID: wsId, TabIds: tabIds}}
}

func TestFindWorkspaceIdForTab(t *testing.T) {
	list := []wshrpc.WorkspaceInfoData{
		makeWorkspaceInfo("ws1", "a", "b"),
		{WorkspaceData: nil},
		makeWorkspaceInfo("ws2", "c"),
	}
	tests := []struct {
		tabId   string
		want    string
		wantErr bool
	}{
		{tabId: "a", want: "ws1"},
		{tabId: "b", want: "ws1"},
		{tabId: "c", want: "ws2"},
		{tabId: "missing", wantErr: true},
		{tabId: "", wantErr: true},
	}
	for _, tt := range tests {
		got, err := findWorkspaceIdForTab(list, tt.tabId)
		if (err != nil) != tt.wantErr {
			t.Fatalf("tab %q: error = %v, wantErr %v", tt.tabId, err, tt.wantErr)
		}
		if got != tt.want {
			t.Fatalf("tab %q: got %q, want %q", tt.tabId, got, tt.want)
		}
	}
}
