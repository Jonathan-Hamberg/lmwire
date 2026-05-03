package main

import (
	"context"
	"net/http"
	"net/http/httptest"
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
