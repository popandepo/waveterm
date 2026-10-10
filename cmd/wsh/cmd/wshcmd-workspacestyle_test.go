// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"reflect"
	"testing"
)

func TestBuildWorkspaceStyleMeta(t *testing.T) {
	got, err := buildWorkspaceStyleMeta("crimson", false)
	if err != nil || !reflect.DeepEqual(got, map[string]any{"ws:accent": "crimson"}) {
		t.Fatalf("got %v, %v", got, err)
	}
	got, err = buildWorkspaceStyleMeta("", true)
	if err != nil || !reflect.DeepEqual(got, map[string]any{"ws:accent": nil}) {
		t.Fatalf("got %v, %v", got, err)
	}
	for _, bad := range [][2]any{{"", false}, {"red", true}, {"notacolour", false}, {"#12345", false}} {
		if _, err := buildWorkspaceStyleMeta(bad[0].(string), bad[1].(bool)); err == nil {
			t.Fatalf("expected an error for %v", bad)
		}
	}
}
