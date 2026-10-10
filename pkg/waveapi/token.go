// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package waveapi

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	TokenFileName  = "api-token"
	SocketFileName = "wave-api.sock"
	TokenBytes     = 32
)

func MakeToken() (string, error) {
	buf := make([]byte, TokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("making token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// The folder is locked to the current user before the token is written, and the file is renamed into
// place, so the token never exists in a place a wider ACL could reach.
func WriteTokenFile(dir string, token string) error {
	tmp, err := os.CreateTemp(dir, "api-token-*.tmp")
	if err != nil {
		return fmt.Errorf("creating token file: %w", err)
	}
	tmpName := tmp.Name()
	_, werr := tmp.WriteString(token)
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		os.Remove(tmpName)
		return fmt.Errorf("writing token file: %v %v", werr, cerr)
	}
	if err := restrictFile(tmpName); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, filepath.Join(dir, TokenFileName)); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("moving token file into place: %w", err)
	}
	return nil
}

func ReadTokenFile(dir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, TokenFileName))
	if err != nil {
		return "", fmt.Errorf("reading token file: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}
