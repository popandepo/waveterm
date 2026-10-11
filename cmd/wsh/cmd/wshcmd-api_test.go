// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"strings"
	"testing"

	"github.com/wavetermdev/waveterm/pkg/waveapi"
)

const (
	apiTestWs  = "11111111-1111-1111-1111-111111111111"
	apiTestTab = "a0000000-0000-0000-0000-00000000000a"
	apiTestBlk = "01000000-0000-0000-0000-000000000001"
)

type fakeApiCaller struct {
	gotoAddr   waveapi.Address
	gotoWindow bool
	focusAddr  waveapi.Address
	calls      []string
}

func (f *fakeApiCaller) List(ctx context.Context) ([]waveapi.Workspace, error) {
	f.calls = append(f.calls, "list")
	return nil, nil
}
func (f *fakeApiCaller) Focus(ctx context.Context, a waveapi.Address) error {
	f.calls = append(f.calls, "focus")
	f.focusAddr = a
	return nil
}
func (f *fakeApiCaller) Goto(ctx context.Context, a waveapi.Address, window bool) error {
	f.calls = append(f.calls, "goto")
	f.gotoAddr = a
	f.gotoWindow = window
	return nil
}

func withFakeApi(t *testing.T) *fakeApiCaller {
	t.Helper()
	fake := &fakeApiCaller{}
	old := makeApiCaller
	makeApiCaller = func(dir string) (apiCaller, error) { return fake, nil }
	oldDir := apiDirFlag
	apiDirFlag = "ignored"
	t.Cleanup(func() { makeApiCaller = old; apiDirFlag = oldDir })
	return fake
}

func TestApiGotoSendsTheAddressAndNoWindowByDefault(t *testing.T) {
	fake := withFakeApi(t)
	apiWorkspace, apiTab, apiBlock, apiWindow = apiTestWs, apiTestTab, apiTestBlk, false
	if err := apiGotoRun(apiGotoCmd, nil); err != nil {
		t.Fatal(err)
	}
	if fake.gotoAddr.WorkspaceId != apiTestWs || fake.gotoAddr.TabId != apiTestTab || fake.gotoAddr.BlockId != apiTestBlk {
		t.Fatalf("address: %+v", fake.gotoAddr)
	}
	if fake.gotoWindow {
		t.Fatalf("the window flag must be off unless asked for")
	}
}

func TestApiGotoWindowFlag(t *testing.T) {
	fake := withFakeApi(t)
	apiWorkspace, apiTab, apiBlock, apiWindow = apiTestWs, apiTestTab, "", true
	if err := apiGotoRun(apiGotoCmd, nil); err != nil {
		t.Fatal(err)
	}
	if !fake.gotoWindow {
		t.Fatalf("window flag not passed")
	}
}

func TestApiGotoWithoutWorkspaceOrTabIsRefusedBeforeTheCall(t *testing.T) {
	fake := withFakeApi(t)
	t.Setenv("WAVETERM_WORKSPACEID", "")
	apiWorkspace, apiTab, apiBlock, apiWindow = "", apiTestTab, "", false
	err := apiGotoRun(apiGotoCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "workspace") {
		t.Fatalf("want a workspace error, got %v", err)
	}
	apiWorkspace, apiTab = apiTestWs, ""
	err = apiGotoRun(apiGotoCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "tab") {
		t.Fatalf("want a tab error, got %v", err)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("calls made: %v", fake.calls)
	}
}

func TestApiGotoUsesTheWorkspaceOfTheCallingBlock(t *testing.T) {
	fake := withFakeApi(t)
	t.Setenv("WAVETERM_WORKSPACEID", apiTestWs)
	apiWorkspace, apiTab, apiBlock, apiWindow = "", apiTestTab, "", false
	if err := apiGotoRun(apiGotoCmd, nil); err != nil {
		t.Fatal(err)
	}
	if fake.gotoAddr.WorkspaceId != apiTestWs {
		t.Fatalf("workspace not taken from the environment: %+v", fake.gotoAddr)
	}
}

func TestApiFocusNeedsABlockOrATab(t *testing.T) {
	fake := withFakeApi(t)
	apiTab, apiBlock = "", ""
	if err := apiFocusRun(apiFocusCmd, nil); err == nil {
		t.Fatalf("want an error without block or tab")
	}
	if len(fake.calls) != 0 {
		t.Fatalf("calls made: %v", fake.calls)
	}
	apiBlock = apiTestBlk
	if err := apiFocusRun(apiFocusCmd, nil); err != nil {
		t.Fatal(err)
	}
	if fake.focusAddr.BlockId != apiTestBlk {
		t.Fatalf("focus address: %+v", fake.focusAddr)
	}
}

func TestBlockStatusTextShowsStatusAndExitCode(t *testing.T) {
	code := 2
	cases := []struct {
		b    waveapi.Block
		want string
	}{
		{waveapi.Block{}, ""},
		{waveapi.Block{Status: "running"}, " running"},
		{waveapi.Block{Status: "notstarted"}, " notstarted"},
		{waveapi.Block{Status: "exited", ExitCode: &code}, " exited(2)"},
	}
	for _, c := range cases {
		if got := blockStatusText(c.b); got != c.want {
			t.Errorf("%+v: want %q, got %q", c.b, c.want, got)
		}
	}
}
