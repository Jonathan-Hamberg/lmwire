package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCLIRenderCommandRemoved(t *testing.T) {
	err := runCLI([]string{"render"})
	if err == nil || !strings.Contains(err.Error(), `unknown command "render"`) {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestRunCLIEnvCommandRemoved(t *testing.T) {
	err := runCLI([]string{"env"})
	if err == nil || !strings.Contains(err.Error(), `unknown command "env"`) {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestAgentCommandOpenCodeUsesProviderModelRefAndInlineConfig(t *testing.T) {
	_, args, envs, err := agentCommand("opencode", Model{
		ProviderID: "lmstudio",
		ID:         "google/gemma-4-e4b",
		Name:       "google/gemma-4-e4b",
		BaseURL:    "http://localhost:1234/v1",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 2 {
		t.Fatalf("unexpected args %#v", args)
	}
	if args[0] != "--model" || args[1] != "lmstudio/google/gemma-4-e4b" {
		t.Fatalf("unexpected args %#v", args)
	}
	if len(envs) != 1 || envs[0].Name != "OPENCODE_CONFIG_CONTENT" {
		t.Fatalf("unexpected envs %#v", envs)
	}
	if !strings.Contains(envs[0].Value, `"lmstudio"`) || !strings.Contains(envs[0].Value, `"google/gemma-4-e4b"`) {
		t.Fatalf("inline config missing provider/model: %s", envs[0].Value)
	}
}

func TestAgentCommandClaudePassesModelFlag(t *testing.T) {
	_, args, envs, err := agentCommand("claude", Model{
		ProviderID: "lmstudio",
		ID:         "openai/gpt-oss-20b",
		Name:       "openai/gpt-oss-20b",
		BaseURL:    "http://localhost:1234/v1",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 2 || args[0] != "--model" || args[1] != "openai/gpt-oss-20b" {
		t.Fatalf("unexpected args %#v", args)
	}
	foundAuthToken := false
	for _, env := range envs {
		if env.Name == "ANTHROPIC_AUTH_TOKEN" && env.Value == "lmstudio" {
			foundAuthToken = true
		}
	}
	if !foundAuthToken {
		t.Fatalf("missing ANTHROPIC_AUTH_TOKEN in %#v", envs)
	}
}

func TestAgentCommandCopilotUsesBYOKEnv(t *testing.T) {
	cmd, args, envs, err := agentCommand("copilot", Model{
		ProviderID: "ollama",
		ID:         "gpt-oss:20b",
		Name:       "gpt-oss:20b",
		BaseURL:    "http://localhost:11434/v1",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cmd != "copilot" {
		t.Fatalf("unexpected command %q", cmd)
	}
	if len(args) != 0 {
		t.Fatalf("unexpected args %#v", args)
	}
	got := map[string]string{}
	for _, env := range envs {
		got[env.Name] = env.Value
	}
	if got["COPILOT_PROVIDER_BASE_URL"] != "http://localhost:11434/v1" {
		t.Fatalf("unexpected base url %q", got["COPILOT_PROVIDER_BASE_URL"])
	}
	if got["COPILOT_PROVIDER_TYPE"] != "openai" {
		t.Fatalf("unexpected provider type %q", got["COPILOT_PROVIDER_TYPE"])
	}
	if got["COPILOT_PROVIDER_MODEL_ID"] != "gpt-oss-20b" {
		t.Fatalf("unexpected model id %q", got["COPILOT_PROVIDER_MODEL_ID"])
	}
	if got["COPILOT_PROVIDER_WIRE_MODEL"] != "gpt-oss:20b" {
		t.Fatalf("unexpected wire model %q", got["COPILOT_PROVIDER_WIRE_MODEL"])
	}
}

func TestAgentCommandCopilotPassesThroughArgs(t *testing.T) {
	_, args, _, err := agentCommand("microsoft-copilot", Model{
		ProviderID: "lmstudio",
		ID:         "openai/gpt-oss-20b",
		Name:       "openai/gpt-oss-20b",
		BaseURL:    "http://localhost:1234/v1",
	}, []string{"--model", "override"})
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 2 || args[0] != "--model" || args[1] != "override" {
		t.Fatalf("unexpected args %#v", args)
	}
}

func TestFilterModelsKeepsSlashesInsideModelID(t *testing.T) {
	models, err := filterModels([]Model{{
		ProviderID: "lmstudio",
		ID:         "google/gemma-4-e4b",
	}}, "lmstudio/google/gemma-4-e4b")
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "google/gemma-4-e4b" {
		t.Fatalf("unexpected models %#v", models)
	}
}

func TestProviderFromModelRef(t *testing.T) {
	got := providerFromModelRef("lmstudio/openai/gpt-oss-20b")
	if got != "lmstudio" {
		t.Fatalf("got %q", got)
	}
}

func TestPrepareAgentRunPiWritesSelectedModelConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	err := prepareAgentRun("pi", Model{
		ProviderID: "ollama",
		ID:         "gemma3:12b",
		Name:       "gemma3:12b",
		BaseURL:    "http://localhost:11434/v1",
	}, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(home, ".pi", "agent", "models.json"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		`"ollama"`,
		`"baseUrl": "http://localhost:11434/v1"`,
		`"id": "gemma3:12b"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in\n%s", want, text)
		}
	}
}
