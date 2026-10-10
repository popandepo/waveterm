// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"reflect"
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

type fakeTabCreator struct {
	calls      []string
	createErr  error
	preloadErr error
	activeErr  error
}

func (f *fakeTabCreator) CreateTab(data wshrpc.CommandCreateTabData) (string, error) {
	f.calls = append(f.calls, fmt.Sprintf("create ws=%s name=%s empty=%v", data.WorkspaceId, data.Name, data.Empty))
	if f.createErr != nil {
		return "", f.createErr
	}
	return "tab-new", nil
}

func (f *fakeTabCreator) PreloadTab(data wshrpc.CommandElectronTabData) error {
	f.calls = append(f.calls, "preload "+data.TabId)
	return f.preloadErr
}

func (f *fakeTabCreator) SetActiveTab(data wshrpc.CommandElectronTabData) error {
	f.calls = append(f.calls, "activate "+data.TabId)
	return f.activeErr
}

func TestCreateBackgroundTabDoesNotActivateByDefault(t *testing.T) {
	f := &fakeTabCreator{}
	id, err := createBackgroundTab(f, "ws1", "bg", true, false)
	if err != nil || id != "tab-new" {
		t.Fatalf("got %q, %v", id, err)
	}
	want := []string{"create ws=ws1 name=bg empty=true", "preload tab-new"}
	if !reflect.DeepEqual(f.calls, want) {
		t.Fatalf("calls = %q, want %q (a tab must be preloaded but never activated unless asked)", f.calls, want)
	}
}

func TestCreateBackgroundTabActivatesOnlyWhenAsked(t *testing.T) {
	f := &fakeTabCreator{}
	if _, err := createBackgroundTab(f, "ws1", "", false, true); err != nil {
		t.Fatal(err)
	}
	want := []string{"create ws=ws1 name= empty=false", "preload tab-new", "activate tab-new"}
	if !reflect.DeepEqual(f.calls, want) {
		t.Fatalf("calls = %q, want %q", f.calls, want)
	}
}

func TestCreateBackgroundTabKeepsTheIdWhenALaterStepFails(t *testing.T) {
	f := &fakeTabCreator{preloadErr: fmt.Errorf("window closed")}
	id, err := createBackgroundTab(f, "ws1", "bg", false, true)
	if err == nil || id != "tab-new" {
		t.Fatalf("preload failure must return the id and an error, got %q, %v", id, err)
	}
	if len(f.calls) != 2 {
		t.Fatalf("must not activate after a failed preload: %q", f.calls)
	}
	f = &fakeTabCreator{createErr: fmt.Errorf("boom")}
	if id, err = createBackgroundTab(f, "ws1", "bg", false, false); err == nil || id != "" {
		t.Fatalf("creation failure returns no id: %q, %v", id, err)
	}
}
