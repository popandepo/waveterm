// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package waveapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

const blkNew = "0e000000-0000-0000-0000-00000000000e"

func (f *fakeBackend) CreateWebBlock(ctx context.Context, tabId string, url string, title string) (string, error) {
	if f.createErr != nil {
		return "", f.createErr
	}
	f.created = append(f.created, tabId+" "+url+" "+title)
	f.calls = append(f.calls, "CreateWebBlock:"+tabId)
	return f.newBlockId, nil
}

func (f *fakeBackend) DeleteBlock(ctx context.Context, blockId string) error {
	f.calls = append(f.calls, "DeleteBlock:"+blockId)
	return nil
}

func newOpenSvc() (*Service, *fakeBackend) {
	svc, fb := newSvc()
	fb.newBlockId = blkNew
	return svc, fb
}

// Without focus the block is made in its tab and nothing else moves: no window, no tab switch, no block focus.
func TestOpenWithoutFocusMovesNothing(t *testing.T) {
	svc, fb := newOpenSvc()
	id, err := svc.Open(context.Background(), OpenRequest{TabId: tabB, Url: "http://127.0.0.1:47475/home/enter?t=x&to=pad", Title: "Tether"})
	if err != nil {
		t.Fatal(err)
	}
	if id != blkNew {
		t.Fatalf("block id %q", id)
	}
	if !reflect.DeepEqual(fb.calls, []string{"CreateWebBlock:" + tabB}) {
		t.Fatalf("calls: %v", fb.calls)
	}
	if !reflect.DeepEqual(fb.created, []string{tabB + " http://127.0.0.1:47475/home/enter?t=x&to=pad Tether"}) {
		t.Fatalf("created: %v", fb.created)
	}
}

// With focus the new block is gone to as goto does with window: the window, then its tab, then the block. The tree is not read
// again, so a block the tree does not list yet (the fake never adds it) is still gone to.
func TestOpenWithFocusGoesToTheBlock(t *testing.T) {
	svc, fb := newOpenSvc()
	if _, err := svc.Open(context.Background(), OpenRequest{TabId: tabB, Url: "https://example.org/", Focus: true}); err != nil {
		t.Fatal(err)
	}
	want := []string{"CreateWebBlock:" + tabB, "FocusWindow:" + win1, "SetActiveTab:" + tabB, "SetBlockFocus:" + blkNew}
	if !reflect.DeepEqual(fb.calls, want) {
		t.Fatalf("calls: %v", fb.calls)
	}
}

// A focus step that fails takes back what open did: the tab it switched to goes back to the tab that was active, and the new
// block is deleted. The error still comes back. A raised window cannot be lowered again; that is the one thing left as it is.
func TestOpenFocusFailureTakesBack(t *testing.T) {
	cases := []struct {
		failOn string
		want   []string
	}{
		{"FocusWindow", []string{"CreateWebBlock:" + tabB, "FocusWindow:" + win1, "DeleteBlock:" + blkNew}},
		{"SetActiveTab", []string{"CreateWebBlock:" + tabB, "FocusWindow:" + win1, "SetActiveTab:" + tabB, "DeleteBlock:" + blkNew}},
		{"SetBlockFocus", []string{"CreateWebBlock:" + tabB, "FocusWindow:" + win1, "SetActiveTab:" + tabB, "SetBlockFocus:" + blkNew, "SetActiveTab:" + tabA, "DeleteBlock:" + blkNew}},
	}
	for _, c := range cases {
		svc, fb := newOpenSvc()
		fb.failOn = c.failOn
		id, err := svc.Open(context.Background(), OpenRequest{TabId: tabB, Url: "https://example.org/", Focus: true})
		if !errors.Is(err, errFocusFailed) || id != "" {
			t.Fatalf("%s: want the focus error and no block id, got %q, %v", c.failOn, id, err)
		}
		if !reflect.DeepEqual(fb.calls, c.want) {
			t.Fatalf("%s: calls: %v", c.failOn, fb.calls)
		}
	}
}

func TestOpenRefusesBeforeAnyCall(t *testing.T) {
	cases := []struct {
		name string
		req  OpenRequest
		want error
	}{
		{"no tab", OpenRequest{Url: "http://127.0.0.1/"}, ErrBadId},
		{"bad tab", OpenRequest{TabId: "TAB", Url: "http://127.0.0.1/"}, ErrBadId},
		{"unknown tab", OpenRequest{TabId: "d0000000-0000-0000-0000-00000000000d", Url: "http://127.0.0.1/"}, ErrNotFound},
		{"no url", OpenRequest{TabId: tabA}, ErrBadUrl},
		{"file url", OpenRequest{TabId: tabA, Url: "file:///C:/Windows/win.ini"}, ErrBadUrl},
		{"script url", OpenRequest{TabId: tabA, Url: "javascript:alert(1)"}, ErrBadUrl},
		{"no host", OpenRequest{TabId: tabA, Url: "http:///home"}, ErrBadUrl},
		{"port but no host", OpenRequest{TabId: tabA, Url: "http://:80/"}, ErrBadUrl},
		{"line separator in url", OpenRequest{TabId: tabA, Url: "http://127.0.0.1/ "}, ErrBadUrl},
		{"title line separator", OpenRequest{TabId: tabA, Url: "http://127.0.0.1/", Title: "a b"}, ErrBadTitle},
		{"title paragraph separator", OpenRequest{TabId: tabA, Url: "http://127.0.0.1/", Title: "a b"}, ErrBadTitle},
		{"user info", OpenRequest{TabId: tabA, Url: "http://user:pw@127.0.0.1/"}, ErrBadUrl},
		{"control char", OpenRequest{TabId: tabA, Url: "http://127.0.0.1/\n"}, ErrBadUrl},
		{"long title", OpenRequest{TabId: tabA, Url: "http://127.0.0.1/", Title: strings.Repeat("t", MaxTitleChars+1)}, ErrBadTitle},
		{"title newline", OpenRequest{TabId: tabA, Url: "http://127.0.0.1/", Title: "a\nb"}, ErrBadTitle},
	}
	for _, c := range cases {
		svc, fb := newOpenSvc()
		_, err := svc.Open(context.Background(), c.req)
		if !errors.Is(err, c.want) {
			t.Fatalf("%s: want %v, got %v", c.name, c.want, err)
		}
		if len(fb.calls) != 0 {
			t.Fatalf("%s: calls made: %v", c.name, fb.calls)
		}
	}
}

// A refused url is never echoed: it can hold an entry token.
func TestOpenErrorNeverHoldsTheUrl(t *testing.T) {
	h, _ := newTestHandler()
	body := `{"tabid":"` + tabA + `","url":"ftp://127.0.0.1/secret-token-value"}`
	rec := doCall(t, h, "POST", "/v1/open", testToken, body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret-token-value") {
		t.Fatalf("the url came back: %s", rec.Body.String())
	}
}

func TestOpenThroughHandler(t *testing.T) {
	svc, fb := newOpenSvc()
	h := MakeHandler(testToken, svc)
	body := `{"tabid":"` + tabB + `","url":"http://127.0.0.1:47475/home/enter","title":"Tether"}`
	rec := doCall(t, h, "POST", "/v1/open", testToken, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		BlockId string `json:"blockid"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp.BlockId != blkNew {
		t.Fatalf("answer %s (%v)", rec.Body.String(), err)
	}
	if !reflect.DeepEqual(fb.calls, []string{"CreateWebBlock:" + tabB}) {
		t.Fatalf("calls: %v", fb.calls)
	}
	for _, bad := range []string{`{"tabid":"` + tabB + `","url":"http://127.0.0.1/","extra":1}`, ``} {
		if rec := doCall(t, h, "POST", "/v1/open", testToken, bad); rec.Code != http.StatusBadRequest {
			t.Fatalf("body %q: want 400, got %d", bad, rec.Code)
		}
	}
	if rec := doCall(t, h, "POST", "/v1/open", "", body); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token: want 401, got %d", rec.Code)
	}
}

func TestOpenCreateFailureIsA500WithoutDetails(t *testing.T) {
	svc, fb := newOpenSvc()
	fb.createErr = errors.New("store said no about http://127.0.0.1/secret")
	h := MakeHandler(testToken, svc)
	rec := doCall(t, h, "POST", "/v1/open", testToken, `{"tabid":"`+tabA+`","url":"http://127.0.0.1/secret"}`)
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
}
