package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func listErrKind(t *testing.T, err error) ListModelsErrorKind {
	t.Helper()
	var le *ListModelsError
	if !errors.As(err, &le) {
		t.Fatalf("err = %v (%T), want *ListModelsError", err, err)
	}
	return le.Kind
}

func TestListModels_Anthropic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/models" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "k" || r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Errorf("headers = %v", r.Header)
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"claude-b"},{"id":"claude-a"}],"has_more":false}`))
	}))
	defer srv.Close()
	got, err := ListModels(context.Background(), ClientConfig{Provider: ProviderDirect, BaseURL: srv.URL, APIKey: "k"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "claude-a" || got[1].ID != "claude-b" {
		t.Errorf("models = %+v, want sorted claude-a, claude-b", got)
	}
}

func TestListModels_AnthropicPaging(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 2 && r.URL.Query().Get("after_id") != "x" {
			t.Errorf("page 2 after_id = %q, want x", r.URL.Query().Get("after_id"))
		}
		if n == 1 {
			_, _ = w.Write([]byte(`{"data":[{"id":"m1"}],"has_more":true,"last_id":"x"}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"m2"}],"has_more":false}`))
	}))
	defer srv.Close()
	got, err := ListModels(context.Background(), ClientConfig{Provider: ProviderDirect, BaseURL: srv.URL, APIKey: "k"}, nil)
	if err != nil || len(got) != 2 {
		t.Fatalf("models = %+v, err = %v, want two pages merged", got, err)
	}

	var endless atomic.Int32
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		endless.Add(1)
		_, _ = w.Write([]byte(`{"data":[{"id":"m"}],"has_more":true,"last_id":"x"}`))
	}))
	defer srv2.Close()
	if _, err := ListModels(context.Background(), ClientConfig{Provider: ProviderDirect, BaseURL: srv2.URL, APIKey: "k"}, nil); err != nil {
		t.Fatal(err)
	}
	if n := endless.Load(); n != 10 {
		t.Errorf("requests = %d, want page cap 10", n)
	}
}

func TestListModels_OpenAI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("request = %s, auth %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"q/b"},{"id":"q/a","context_length":262144}]}`))
	}))
	defer srv.Close()
	got, err := ListModels(context.Background(), ClientConfig{Provider: ProviderOpenAI, BaseURL: srv.URL, APIKey: "k"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != (ModelListing{ID: "q/a", ContextWindow: 262144}) || got[1] != (ModelListing{ID: "q/b"}) {
		t.Errorf("models = %+v", got)
	}
}

func TestListModels_BaseURLWithV1(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("path = %s, want /v1/models", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()
	if _, err := ListModels(context.Background(), ClientConfig{Provider: ProviderOpenAI, BaseURL: srv.URL + "/v1", APIKey: "k"}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestListModels_ErrorKinds(t *testing.T) {
	cases := []struct {
		status int
		body   string
		want   ListModelsErrorKind
	}{
		{401, `{"error":"bad key"}`, ListModelsUnauthorized},
		{403, `{}`, ListModelsForbidden},
		{404, `not found`, ListModelsNotSupported},
		{500, `oops`, ListModelsHTTP},
		{200, `<html>`, ListModelsNotSupported},
	}
	for _, c := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(c.status)
			_, _ = w.Write([]byte(c.body))
		}))
		_, err := ListModels(context.Background(), ClientConfig{Provider: ProviderOpenAI, BaseURL: srv.URL, APIKey: "k"}, nil)
		srv.Close()
		if got := listErrKind(t, err); got != c.want {
			t.Errorf("status %d: kind %s, want %s", c.status, got, c.want)
		}
	}

	closed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := closed.URL
	closed.Close()
	_, err := ListModels(context.Background(), ClientConfig{Provider: ProviderOpenAI, BaseURL: url, APIKey: "k"}, nil)
	if got := listErrKind(t, err); got != ListModelsUnreachable {
		t.Errorf("closed server: kind %s, want unreachable", got)
	}

	slow := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer slow.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = ListModels(ctx, ClientConfig{Provider: ProviderOpenAI, BaseURL: slow.URL, APIKey: "k"}, nil)
	if got := listErrKind(t, err); got != ListModelsTimeout {
		t.Errorf("slow server: kind %s, want timeout", got)
	}

	var srv500 *httptest.Server
	srv500 = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(500) }))
	defer srv500.Close()
	_, err = ListModels(context.Background(), ClientConfig{Provider: ProviderOpenAI, BaseURL: srv500.URL, APIKey: "k"}, nil)
	var le *ListModelsError
	if !errors.As(err, &le) || le.StatusCode != 500 {
		t.Errorf("StatusCode = %+v, want 500", le)
	}
}

func TestListModels_ErrorRedactsKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`invalid header Authorization: Bearer sk-secret-123 (key sk-secret-123)`))
	}))
	defer srv.Close()
	_, err := ListModels(context.Background(), ClientConfig{Provider: ProviderOpenAI, BaseURL: srv.URL, APIKey: "sk-secret-123"}, nil)
	if err == nil || strings.Contains(err.Error(), "sk-secret-123") {
		t.Errorf("error leaks the key: %v", err)
	}
}

func TestListModels_UnsupportedProvider(t *testing.T) {
	_, err := ListModels(context.Background(), ClientConfig{Provider: ProviderBedrock, APIKey: "k"}, nil)
	if got := listErrKind(t, err); got != ListModelsNotSupported {
		t.Errorf("bedrock: kind %s, want not_supported", got)
	}
}
