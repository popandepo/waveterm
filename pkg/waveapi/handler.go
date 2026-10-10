// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package waveapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
)

const (
	MaxBodyBytes = 1 << 20
	BearerPrefix = "Bearer "
)

type gotoRequest struct {
	Address
	Window bool `json:"window,omitempty"`
}

type errorResponse struct {
	Error string `json:"error"`
}

type listResponse struct {
	Workspaces []Workspace `json:"workspaces"`
}

type okResponse struct {
	Ok bool `json:"ok"`
}

// the digests have equal length, so the comparison time does not depend on how much of the token matched
func tokenMatches(want string, got string) bool {
	a := sha256.Sum256([]byte(want))
	b := sha256.Sum256([]byte(got))
	return subtle.ConstantTimeCompare(a[:], b[:]) == 1
}

func writeJson(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func statusForError(err error) int {
	switch {
	case errors.Is(err, ErrBadId):
		return http.StatusBadRequest
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrMismatch), errors.Is(err, ErrNotActive):
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

func writeError(w http.ResponseWriter, err error) {
	status := statusForError(err)
	if status == http.StatusInternalServerError {
		log.Printf("waveapi: internal error: %v\n", err)
		writeJson(w, status, errorResponse{Error: "internal error"})
		return
	}
	writeJson(w, status, errorResponse{Error: err.Error()})
}

func readLimitedBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxBodyBytes))
	if err == nil {
		return data, true
	}
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		writeJson(w, http.StatusRequestEntityTooLarge, errorResponse{Error: "body too large"})
		return nil, false
	}
	writeJson(w, http.StatusBadRequest, errorResponse{Error: "could not read body"})
	return nil, false
}

// The whole body is read inside the limit and must be exactly one JSON value; Unmarshal refuses
// anything after it, so a request cannot change state and then carry unread bytes.
func readBody(w http.ResponseWriter, r *http.Request, v any) bool {
	data, ok := readLimitedBody(w, r)
	if !ok {
		return false
	}
	if err := json.Unmarshal(data, v); err != nil {
		writeJson(w, http.StatusBadRequest, errorResponse{Error: "body is not exactly one json value"})
		return false
	}
	return true
}

func MakeHandler(token string, svc *Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/list", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := readLimitedBody(w, r); !ok {
			return
		}
		tree, err := svc.List(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJson(w, http.StatusOK, listResponse{Workspaces: tree})
	})
	mux.HandleFunc("/v1/focus", func(w http.ResponseWriter, r *http.Request) {
		var req Address
		if !readBody(w, r, &req) {
			return
		}
		if err := svc.Focus(r.Context(), req); err != nil {
			writeError(w, err)
			return
		}
		writeJson(w, http.StatusOK, okResponse{Ok: true})
	})
	mux.HandleFunc("/v1/goto", func(w http.ResponseWriter, r *http.Request) {
		var req gotoRequest
		if !readBody(w, r, &req) {
			return
		}
		if err := svc.Goto(r.Context(), req.Address, req.Window); err != nil {
			writeError(w, err)
			return
		}
		writeJson(w, http.StatusOK, okResponse{Ok: true})
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, BearerPrefix) || !tokenMatches(token, strings.TrimPrefix(auth, BearerPrefix)) {
			writeJson(w, http.StatusUnauthorized, errorResponse{Error: "unauthorized"})
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJson(w, http.StatusMethodNotAllowed, errorResponse{Error: "use POST"})
			return
		}
		mux.ServeHTTP(w, r)
	})
}
