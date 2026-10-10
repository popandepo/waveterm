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

type Server struct {
	Dir      string
	listener net.Listener
	httpSrv  *http.Server
}

// A new token is written at every start, so a token from an earlier run stops working.
func StartServer(dir string, svc *Service) (*Server, error) {
	if err := PrepareApiDir(dir); err != nil {
		return nil, err
	}
	token, err := MakeToken()
	if err != nil {
		return nil, err
	}
	if err := WriteTokenFile(dir, token); err != nil {
		return nil, err
	}
	sockPath := filepath.Join(dir, SocketFileName)
	os.Remove(sockPath)
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		return nil, fmt.Errorf("listening on %s: %w", sockPath, err)
	}
	httpSrv := &http.Server{Handler: MakeHandler(token, svc), ReadHeaderTimeout: 5 * time.Second}
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
