// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package waveapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"time"
)

type Client struct {
	Dir   string
	Token string
}

func MakeClient(dir string) (*Client, error) {
	token, err := ReadTokenFile(dir)
	if err != nil {
		return nil, err
	}
	return &Client{Dir: dir, Token: token}, nil
}

func (c *Client) call(ctx context.Context, path string, in any, out any) error {
	var body io.Reader = http.NoBody
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	sockPath := filepath.Join(c.Dir, SocketFileName)
	hc := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network string, addr string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", sockPath)
			},
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://wave-api"+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", BearerPrefix+c.Token)
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("calling wave api: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxBodyBytes*8))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		var e errorResponse
		json.Unmarshal(data, &e)
		return fmt.Errorf("wave api %d: %s", resp.StatusCode, e.Error)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

func (c *Client) List(ctx context.Context) ([]Workspace, error) {
	var resp listResponse
	if err := c.call(ctx, "/v1/list", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Workspaces, nil
}

func (c *Client) Focus(ctx context.Context, a Address) error {
	return c.call(ctx, "/v1/focus", a, nil)
}

func (c *Client) Goto(ctx context.Context, a Address, window bool) error {
	return c.call(ctx, "/v1/goto", gotoRequest{Address: a, Window: window}, nil)
}
