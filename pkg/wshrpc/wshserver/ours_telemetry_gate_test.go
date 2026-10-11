// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/wavetermdev/waveterm/pkg/telemetry"
	"github.com/wavetermdev/waveterm/pkg/telemetry/telemetrydata"
	"github.com/wavetermdev/waveterm/pkg/wavebase"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wcloud"
	"github.com/wavetermdev/waveterm/pkg/wconfig"
	"github.com/wavetermdev/waveterm/pkg/wcore"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wstore"
)

type gateTransport struct {
	mu    sync.Mutex
	calls []string
}

func (c *gateTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c.mu.Lock()
	c.calls = append(c.calls, req.Method+" "+req.URL.Host+req.URL.Path)
	c.mu.Unlock()
	return nil, errors.New("no network in this test")
}

func (c *gateTransport) list() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string{}, c.calls...)
}

func (c *gateTransport) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = nil
}

// The refusal comes first in SetConfigCommand, so a refused key never reaches the file.
func TestSetConfigCommandRefusesTheOwnerOnlyKeys(t *testing.T) {
	ws := &WshServer{}
	for _, key := range []string{"telemetry:enabled", "autoupdate:enabled", "autoupdate:channel", "telemetry:*", "autoupdate:*"} {
		err := ws.SetConfigCommand(context.Background(), wshrpc.MetaSettingsType{MetaMapType: waveobj.MetaMapType{key: true}})
		if err == nil || !strings.Contains(err.Error(), wconfig.OwnerOnlyKeysRefused) {
			t.Fatalf("SetConfigCommand(%s) must be refused with the stated reason, got %v", key, err)
		}
	}
	src, rerr := os.ReadFile("wshserver.go")
	if rerr != nil {
		t.Fatal(rerr)
	}
	text := string(src)
	i := strings.Index(text, "func (ws *WshServer) SetConfigCommand(ctx context.Context, data wshrpc.MetaSettingsType) error {")
	if i < 0 {
		t.Fatal("handler not found")
	}
	body := strings.TrimSpace(text[i+len("func (ws *WshServer) SetConfigCommand(ctx context.Context, data wshrpc.MetaSettingsType) error {"):])
	if !strings.HasPrefix(body, "if err := wconfig.CheckNoOwnerOnlyKeys(data.MetaMapType); err != nil {") {
		t.Fatalf("the handler must refuse before anything else, it starts with %q", body[:80])
	}
}

// One process, one settings file (the config watcher reads it once): telemetry is OFF, as on a fresh profile. An un-uploaded usage event is put into the store
// directly (the recording function drops events while telemetry is off), so a sender that lost its gate would have something to send.
func TestWithTelemetryOffNothingIsRecordedOrSent(t *testing.T) {
	ct := &gateTransport{}
	oldClient, oldDefault := http.DefaultClient.Transport, http.DefaultTransport
	http.DefaultClient.Transport, http.DefaultTransport = ct, ct
	t.Cleanup(func() { http.DefaultClient.Transport, http.DefaultTransport = oldClient, oldDefault })

	dataDir, configDir := t.TempDir(), t.TempDir()
	wavebase.DataHome_VarCache = dataDir
	wavebase.ConfigHome_VarCache = configDir
	if err := os.MkdirAll(filepath.Join(dataDir, wavebase.WaveDBDir), 0700); err != nil {
		t.Fatal(err)
	}
	if err := wstore.InitWStore(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = wstore.CloseDB() })
	if telemetry.IsTelemetryEnabled() {
		t.Fatal("telemetry is on in a profile without a settings file")
	}
	ctx := context.Background()

	// the control: the stub does see a sender that is live, so "0 requests" below means something
	if err := wcloud.SendDiagnosticPing(ctx, "client-id-1", true); err == nil && len(ct.list()) == 0 {
		t.Fatal("the control sender did not reach the stub")
	}
	if len(ct.list()) != 1 {
		t.Fatalf("the control must make exactly one request, got %v", ct.list())
	}
	ct.reset()

	now := time.Now()
	err := wstore.WithTx(ctx, func(tx *wstore.TxWrap) error {
		tx.Exec(`INSERT INTO db_tevent (uuid, ts, tslocal, event, props) VALUES (?, ?, ?, ?, ?)`, uuid.New().String(), now.UnixMilli(), now.Format(time.RFC3339), "app:startup", "{}")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	before, err := telemetry.GetNonUploadedTEvents(ctx, 10)
	if err != nil || len(before) != 1 {
		t.Fatalf("the set-up event is not in the store: %v %v", len(before), err)
	}

	// recording while off drops the event
	ev := telemetrydata.MakeTEvent("app:activity", telemetrydata.TEventProps{})
	if err := telemetry.RecordTEvent(ctx, ev); err != nil {
		t.Fatalf("RecordTEvent while off must not fail: %v", err)
	}
	after, err := telemetry.GetNonUploadedTEvents(ctx, 10)
	if err != nil || len(after) != 1 {
		t.Fatalf("an event was recorded with telemetry off: %d events in the store (%v)", len(after), err)
	}

	// sending: the central gate and both directions of the no-telemetry update
	if err := wcloud.SendAllTelemetry("client-id-1"); err != nil {
		t.Fatalf("SendAllTelemetry with telemetry off must succeed without sending: %v", err)
	}
	wcore.GoSendNoTelemetryUpdate(true)
	wcore.GoSendNoTelemetryUpdate(false)
	time.Sleep(700 * time.Millisecond)
	if calls := ct.list(); len(calls) != 0 {
		t.Fatalf("with telemetry off something was sent: %v", calls)
	}
}
