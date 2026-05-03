package main

import (
	"strings"
	"testing"
)

func TestRenderCodexUsesManagedBlockAndCustomProvider(t *testing.T) {
	models := []Model{{
		ProviderID: "ollama",
		ID:         "qwen2.5-coder:7b",
		Name:       "qwen2.5-coder:7b",
		BaseURL:    "http://localhost:11434/v1",
	}}
	patch, err := renderCodex(models)
	if err != nil {
		t.Fatal(err)
	}
	text := string(patch.After)
	for _, want := range []string{
		"# lmwire managed begin",
		"[model_providers.lmwire_ollama]",
		`base_url = "http://localhost:11434/v1"`,
		"[profiles.lmwire_ollama_qwen2_5_coder_7b]",
		`model = "qwen2.5-coder:7b"`,
		"model_context_window = 4096",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in\n%s", want, text)
		}
	}
}

func TestRenderCodexUsesDiscoveredContextLength(t *testing.T) {
	patch, err := renderCodexWithContext([]Model{{
		ProviderID: "lmstudio",
		ID:         "openai/gpt-oss-20b",
		Name:       "openai/gpt-oss-20b",
		BaseURL:    "http://localhost:1234/v1",
		Metadata:   map[string]string{"context_length": "32768"},
	}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(patch.After), "model_context_window = 32768") {
		t.Fatalf("missing discovered context window in\n%s", patch.After)
	}
}

func TestRenderClaudeEnv(t *testing.T) {
	envs := renderClaudeEnv(Model{
		ProviderID: "ollama",
		ID:         "qwen3.5",
		Name:       "qwen3.5",
		BaseURL:    "http://localhost:11434/v1",
	})
	got := map[string]string{}
	for _, env := range envs {
		got[env.Name] = env.Value
	}
	if got["ANTHROPIC_BASE_URL"] != "http://localhost:11434" {
		t.Fatalf("unexpected base url %q", got["ANTHROPIC_BASE_URL"])
	}
	if got["ANTHROPIC_AUTH_TOKEN"] != "ollama" {
		t.Fatalf("unexpected auth token %q", got["ANTHROPIC_AUTH_TOKEN"])
	}
	if got["ANTHROPIC_API_KEY"] != "" {
		t.Fatalf("unexpected api key %q", got["ANTHROPIC_API_KEY"])
	}
	if got["ANTHROPIC_MODEL"] != "qwen3.5" {
		t.Fatalf("unexpected model %q", got["ANTHROPIC_MODEL"])
	}
}

func TestRenderClaudeEnvStripsLMStudioOpenAIPath(t *testing.T) {
	envs := renderClaudeEnv(Model{
		ProviderID: "lmstudio",
		ID:         "openai/gpt-oss-20b",
		Name:       "openai/gpt-oss-20b",
		BaseURL:    "http://localhost:1234/v1",
	})
	got := map[string]string{}
	for _, env := range envs {
		got[env.Name] = env.Value
	}
	if got["ANTHROPIC_BASE_URL"] != "http://localhost:1234" {
		t.Fatalf("unexpected base url %q", got["ANTHROPIC_BASE_URL"])
	}
	if got["ANTHROPIC_AUTH_TOKEN"] != "lmstudio" {
		t.Fatalf("unexpected auth token %q", got["ANTHROPIC_AUTH_TOKEN"])
	}
}

func TestSanitizeID(t *testing.T) {
	got := sanitizeID("ollama_qwen2.5-coder:7b")
	want := "ollama_qwen2_5_coder_7b"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
