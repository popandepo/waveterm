// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package waveapi

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"slices"
	"strings"
)

const (
	MaxBodyBytes  = 1 << 20
	BearerPrefix  = "Bearer "
	MaxErrorChars = 64
)

var (
	addressKeys = []string{"workspaceid", "tabid", "blockid"}
	gotoKeys    = []string{"workspaceid", "tabid", "blockid", "window"}
	screenKeys  = []string{"blockid"}
	sendKeys    = []string{"blockid", "expectsession", "text"}
)

func truncateForError(s string) string {
	if len(s) <= MaxErrorChars {
		return s
	}
	return s[:MaxErrorChars] + "..."
}

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
	if want == "" {
		return false
	}
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
	case errors.Is(err, ErrRefusedText):
		return http.StatusBadRequest
	case errors.Is(err, ErrMismatch), errors.Is(err, ErrNotActive), errors.Is(err, ErrWrongSession), errors.Is(err, ErrNotRunning),
		errors.Is(err, ErrBusy), errors.Is(err, ErrOnPrompt), errors.Is(err, ErrNoInputBox), errors.Is(err, ErrNotSent):
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

// The whole body is read inside the limit and must be exactly one JSON object whose keys are all known,
// spelled exactly, and appear once. Go's decoder alone would take the first value only, fold the case of
// keys and let the last duplicate win; each of those lets a request mean something other than what a
// reader of the log sees.
func strictDecode(data []byte, v any, allowed []string) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return fmt.Errorf("body is not a json object")
	}
	seen := map[string]bool{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return fmt.Errorf("body is not valid json")
		}
		key, _ := keyTok.(string)
		if !slices.Contains(allowed, key) {
			return fmt.Errorf("unknown key %q", truncateForError(key))
		}
		if seen[key] {
			return fmt.Errorf("key %q appears twice", key)
		}
		seen[key] = true
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return fmt.Errorf("body is not valid json")
		}
	}
	if _, err := dec.Token(); err != nil {
		return fmt.Errorf("body is not valid json")
	}
	if _, err := dec.Token(); err != io.EOF {
		return fmt.Errorf("body has more than one json value")
	}
	typed := json.NewDecoder(bytes.NewReader(data))
	typed.DisallowUnknownFields()
	if err := typed.Decode(v); err != nil {
		return fmt.Errorf("body does not fit the call: %v", truncateForError(err.Error()))
	}
	return nil
}

func readBody(w http.ResponseWriter, r *http.Request, v any, allowed []string) bool {
	data, ok := readLimitedBody(w, r)
	if !ok {
		return false
	}
	if err := strictDecode(data, v, allowed); err != nil {
		writeJson(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return false
	}
	return true
}

func MakeHandler(token string, svc *Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/list", func(w http.ResponseWriter, r *http.Request) {
		var none struct{}
		data, ok := readLimitedBody(w, r)
		if !ok {
			return
		}
		if len(bytes.TrimSpace(data)) > 0 {
			if err := strictDecode(data, &none, nil); err != nil {
				writeJson(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
				return
			}
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
		if !readBody(w, r, &req, addressKeys) {
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
		if !readBody(w, r, &req, gotoKeys) {
			return
		}
		if err := svc.Goto(r.Context(), req.Address, req.Window); err != nil {
			writeError(w, err)
			return
		}
		writeJson(w, http.StatusOK, okResponse{Ok: true})
	})
	mux.HandleFunc("/v1/screen", func(w http.ResponseWriter, r *http.Request) {
		var req screenRequest
		if !readBody(w, r, &req, screenKeys) {
			return
		}
		lines, err := svc.ScreenOf(r.Context(), req.BlockId)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJson(w, http.StatusOK, screenResponse{Lines: lines})
	})
	// The answer never holds screen text, whatever happened.
	mux.HandleFunc("/v1/send", func(w http.ResponseWriter, r *http.Request) {
		var req SendRequest
		if !readBody(w, r, &req, sendKeys) {
			return
		}
		if err := svc.Send(r.Context(), req); err != nil {
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
