// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestTheAIPanelCannotSwitchTelemetryOn(t *testing.T) {
	ws := &WshServer{}
	err := ws.WaveAIEnableTelemetryCommand(context.Background())
	if err == nil || err.Error() != WaveAIEnableTelemetryRefused {
		t.Fatalf("the request must be refused with the stated reason, got %v", err)
	}
	src, rerr := os.ReadFile("wshserver.go")
	if rerr != nil {
		t.Fatal(rerr)
	}
	text := string(src)
	i := strings.Index(text, "func (ws *WshServer) WaveAIEnableTelemetryCommand(ctx context.Context) error {")
	if i < 0 {
		t.Fatal("handler not found")
	}
	first := text[i:]
	first = first[strings.Index(first, "{")+1:]
	first = strings.TrimSpace(first[:strings.Index(first, "}")])
	if !strings.HasPrefix(first, "return fmt.Errorf") || strings.Contains(first, "SetBaseConfigValue") {
		t.Fatalf("the handler must refuse before it touches the config, its first statement is %q", first)
	}
}
