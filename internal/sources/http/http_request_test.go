// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package http

import (
	"bytes"
	"context"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/googleapis/genai-toolbox/internal/log"
	"github.com/googleapis/genai-toolbox/internal/util"
)

func TestRunRequestSanitizesErrorBodyByDefault(t *testing.T) {
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		w.WriteHeader(nethttp.StatusBadRequest)
		_, _ = w.Write([]byte("sensitive details"))
	}))
	defer server.Close()

	source := &Source{
		Config: Config{},
		client: server.Client(),
	}

	logger, err := log.NewLogger("standard", log.Debug, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	ctx := util.WithLogger(context.Background(), logger)

	req, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}

	_, err = source.RunRequest(ctx, req)
	if err == nil {
		t.Fatalf("expected error for non-2xx response")
	}
	if strings.Contains(err.Error(), "sensitive details") {
		t.Fatalf("expected sanitized error message, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "unexpected status code: 400") {
		t.Fatalf("expected status code in error message, got %q", err.Error())
	}
}

func TestRunRequestIncludesErrorBodyWhenEnabled(t *testing.T) {
	server := httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		w.WriteHeader(nethttp.StatusInternalServerError)
		_, _ = w.Write([]byte("sensitive details"))
	}))
	defer server.Close()

	source := &Source{
		Config: Config{ReturnFullError: true},
		client: server.Client(),
	}

	logger, err := log.NewLogger("standard", log.Debug, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	ctx := util.WithLogger(context.Background(), logger)

	req, err := nethttp.NewRequestWithContext(ctx, nethttp.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}

	_, err = source.RunRequest(ctx, req)
	if err == nil {
		t.Fatalf("expected error for non-2xx response")
	}
	if !strings.Contains(err.Error(), "response body: sensitive details") {
		t.Fatalf("expected response body in error message, got %q", err.Error())
	}
}
