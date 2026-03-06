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
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

	req, err := nethttp.NewRequest(nethttp.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}

	_, err = source.RunRequest(req)
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
		Config: Config{IncludeResponseBodyInErrors: true},
		client: server.Client(),
	}

	req, err := nethttp.NewRequest(nethttp.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}

	_, err = source.RunRequest(req)
	if err == nil {
		t.Fatalf("expected error for non-2xx response")
	}
	if !strings.Contains(err.Error(), "response body: sensitive details") {
		t.Fatalf("expected response body in error message, got %q", err.Error())
	}
}
