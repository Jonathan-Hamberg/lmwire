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
		"# ai_config managed begin",
		"[model_providers.ai_config_ollama]",
		`base_url = "http://localhost:11434/v1"`,
		"[profiles.ai_config_ollama_qwen2_5_coder_7b]",
		`model = "qwen2.5-coder:7b"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in\n%s", want, text)
		}
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
	if got["ANTHROPIC_MODEL"] != "qwen3.5" {
		t.Fatalf("unexpected model %q", got["ANTHROPIC_MODEL"])
	}
}

func TestSanitizeID(t *testing.T) {
	got := sanitizeID("ollama_qwen2.5-coder:7b")
	want := "ollama_qwen2_5_coder_7b"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
