package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDiscoverOllamaHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Write([]byte(`{"models":[{"name":"qwen2.5-coder:7b","model":"qwen2.5-coder:7b","details":{"family":"qwen2","parameter_size":"7B","quantization_level":"Q4_K_M"}}]}`))
	}))
	defer server.Close()

	models, err := discoverOllamaHTTP(context.Background(), Provider{
		ID:      "ollama",
		BaseURL: server.URL + "/v1",
		ListURL: server.URL + "/api/tags",
	}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}
	if models[0].ID != "qwen2.5-coder:7b" {
		t.Fatalf("unexpected model id %q", models[0].ID)
	}
	if models[0].Metadata["family"] != "qwen2" {
		t.Fatalf("unexpected metadata %#v", models[0].Metadata)
	}
}

func TestDiscoverOpenAIModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Write([]byte(`{"object":"list","data":[{"id":"google/gemma-3n-e4b"}]}`))
	}))
	defer server.Close()

	models, err := discoverOpenAIModels(context.Background(), Provider{
		ID:      "lmstudio",
		BaseURL: server.URL + "/v1",
		ListURL: server.URL + "/v1/models",
	}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}
	if models[0].ProviderID != "lmstudio" || models[0].ID != "google/gemma-3n-e4b" {
		t.Fatalf("unexpected model %#v", models[0])
	}
}

func TestDiscoverLMStudioNativeMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/models" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Write([]byte(`{"models":[{"type":"llm","key":"openai/gpt-oss-20b","display_name":"GPT OSS 20B","architecture":"gpt-oss","params_string":"20B","max_context_length":131072,"loaded_instances":[{"id":"openai/gpt-oss-20b","config":{"context_length":32768}}]}]}`))
	}))
	defer server.Close()

	meta, err := discoverLMStudioNativeMetadata(context.Background(), Provider{
		ID:        "lmstudio",
		NativeURL: server.URL + "/api/v1/models",
	}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	got := meta["openai/gpt-oss-20b"]["context_length"]
	if got != "32768" {
		t.Fatalf("context_length = %q", got)
	}
	if meta["openai/gpt-oss-20b"]["max_context_length"] != "131072" {
		t.Fatalf("unexpected metadata %#v", meta["openai/gpt-oss-20b"])
	}
}

func TestRequireOllamaAnthropicCompatibilityRejectsOldVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/version" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Write([]byte(`{"version":"0.13.3"}`))
	}))
	defer server.Close()

	err := requireOllamaAnthropicCompatibility(context.Background(), Model{
		ProviderID: "ollama",
		ID:         "gpt-oss:20b",
		BaseURL:    server.URL + "/v1",
	}, time.Second)
	if err == nil || !strings.Contains(err.Error(), "upgrade Ollama to 0.14.0 or newer") {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestRequireOllamaAnthropicCompatibilityAcceptsNewVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/version" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Write([]byte(`{"version":"0.14.0"}`))
	}))
	defer server.Close()

	err := requireOllamaAnthropicCompatibility(context.Background(), Model{
		ProviderID: "ollama",
		ID:         "gpt-oss:20b",
		BaseURL:    server.URL + "/v1",
	}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
}

func TestVersionAtLeast(t *testing.T) {
	for _, tc := range []struct {
		got  string
		want bool
	}{
		{"0.13.3", false},
		{"0.14.0", true},
		{"0.14.1", true},
		{"1.0.0", true},
		{"0.14.0-rc1", true},
	} {
		if versionAtLeast(tc.got, "0.14.0") != tc.want {
			t.Fatalf("versionAtLeast(%q) = %v", tc.got, !tc.want)
		}
	}
}
