// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package waveapi

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	MaxSocketPathChars     = 100
	liveSocketProbeTimeout = 500 * time.Millisecond
	readHeaderTimeout      = 5 * time.Second
	readTimeout            = 15 * time.Second
	writeTimeout           = 15 * time.Second
	idleTimeout            = 30 * time.Second
	maxHeaderBytes         = 64 << 10
)

// These are variables only so tests can make each step fail; production code never reassigns them.
var (
	prepareDirFn    = PrepareApiDir
	verifyPrivateFn = VerifyPrivate
	writeTokenFn    = WriteTokenFile
)

type Server struct {
	Dir      string
	listener net.Listener
	httpSrv  *http.Server
}

func socketIsLive(sockPath string) bool {
	conn, err := net.DialTimeout("unix", sockPath, liveSocketProbeTimeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// Order matters: the folder is locked and checked first; nothing is created in it before that. A server
// that already listens on the socket is left alone. The token is written only after the socket is listening
// and private, and a new token is written at every start, so a token from an earlier run stops working.
func StartServer(dir string, svc *Service) (*Server, error) {
	if err := prepareDirFn(dir); err != nil {
		return nil, err
	}
	sockPath := filepath.Join(dir, SocketFileName)
	if len(sockPath) > MaxSocketPathChars {
		return nil, fmt.Errorf("socket path is too long (%d characters, the limit is %d): %s", len(sockPath), MaxSocketPathChars, sockPath)
	}
	if socketIsLive(sockPath) {
		return nil, fmt.Errorf("another server already listens on %s", sockPath)
	}
	os.Remove(sockPath)
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		return nil, fmt.Errorf("listening on %s: %w", sockPath, err)
	}
	abort := func() {
		listener.Close()
		os.Remove(sockPath)
	}
	if err := restrictSocket(sockPath); err != nil {
		abort()
		return nil, fmt.Errorf("locking socket: %w", err)
	}
	if err := verifyPrivateFn(sockPath, false); err != nil {
		abort()
		return nil, fmt.Errorf("socket is not private: %w", err)
	}
	token, err := MakeToken()
	if err != nil {
		abort()
		return nil, err
	}
	if err := writeTokenFn(dir, token); err != nil {
		abort()
		return nil, err
	}
	httpSrv := &http.Server{
		Handler:           MakeHandler(token, svc),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
	}
	srv := &Server{Dir: dir, listener: listener, httpSrv: httpSrv}
	go func() {
		err := httpSrv.Serve(listener)
		if err != nil && err != http.ErrServerClosed {
			log.Printf("waveapi: server stopped: %v\n", err)
		}
	}()
	return srv, nil
}

func (s *Server) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	s.httpSrv.Shutdown(ctx)
	s.listener.Close()
	os.Remove(filepath.Join(s.Dir, SocketFileName))
}
