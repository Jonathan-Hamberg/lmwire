package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

var defaultProviders = []Provider{
	{
		ID:          "ollama",
		DisplayName: "Ollama",
		BaseURL:     "http://localhost:11434/v1",
		ListURL:     "http://localhost:11434/api/tags",
		Kind:        "ollama",
	},
	{
		ID:          "lmstudio",
		DisplayName: "LM Studio",
		BaseURL:     "http://localhost:1234/v1",
		ListURL:     "http://localhost:1234/v1/models",
		NativeURL:   "http://localhost:1234/api/v1/models",
		Kind:        "openai-models",
	},
}

func discoverModels(ctx context.Context, opts DiscoverOptions) ([]Model, []error) {
	providers := selectProviders(opts.Providers)
	var models []Model
	var errs []error
	if len(providers) == 0 {
		return nil, []error{fmt.Errorf("no matching providers for %q", strings.Join(opts.Providers, ","))}
	}
	for _, provider := range providers {
		found, err := discoverProvider(ctx, provider, opts.Timeout)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", provider.ID, err))
			continue
		}
		models = append(models, found...)
	}
	return models, errs
}

func selectProviders(ids []string) []Provider {
	if len(ids) == 0 {
		return defaultProviders
	}
	want := map[string]bool{}
	for _, id := range ids {
		want[strings.ToLower(strings.TrimSpace(id))] = true
	}
	var selected []Provider
	for _, provider := range defaultProviders {
		if want[provider.ID] {
			selected = append(selected, provider)
		}
	}
	return selected
}

func discoverProvider(ctx context.Context, provider Provider, timeout time.Duration) ([]Model, error) {
	switch provider.Kind {
	case "ollama":
		models, err := discoverOllamaHTTP(ctx, provider, timeout)
		if err == nil {
			return models, nil
		}
		return discoverOllamaCLI(ctx, provider)
	case "openai-models":
		return discoverOpenAIModels(ctx, provider, timeout)
	default:
		return nil, fmt.Errorf("unknown provider kind %q", provider.Kind)
	}
}

func discoverOllamaHTTP(ctx context.Context, provider Provider, timeout time.Duration) ([]Model, error) {
	var body struct {
		Models []struct {
			Name    string `json:"name"`
			Model   string `json:"model"`
			Details struct {
				Family            string   `json:"family"`
				Families          []string `json:"families"`
				ParameterSize     string   `json:"parameter_size"`
				QuantizationLevel string   `json:"quantization_level"`
			} `json:"details"`
		} `json:"models"`
	}
	if err := getJSON(ctx, provider.ListURL, timeout, &body); err != nil {
		return nil, err
	}
	var models []Model
	for _, item := range body.Models {
		id := firstNonEmpty(item.Model, item.Name)
		if id == "" {
			continue
		}
		meta := map[string]string{
			"family":       item.Details.Family,
			"parameters":   item.Details.ParameterSize,
			"quantization": item.Details.QuantizationLevel,
		}
		models = append(models, Model{
			ProviderID: provider.ID,
			ID:         id,
			Name:       id,
			BaseURL:    provider.BaseURL,
			API:        "openai-chat",
			Metadata:   compactMetadata(meta),
		})
	}
	return models, nil
}

func discoverOllamaCLI(ctx context.Context, provider Provider) ([]Model, error) {
	cmd := exec.CommandContext(ctx, "ollama", "list")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(out), "\n")
	var models []Model
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || i == 0 && strings.HasPrefix(strings.ToLower(line), "name") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		models = append(models, Model{
			ProviderID: provider.ID,
			ID:         fields[0],
			Name:       fields[0],
			BaseURL:    provider.BaseURL,
			API:        "openai-chat",
			Metadata:   map[string]string{"source": "ollama list"},
		})
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("no models returned by ollama list")
	}
	return models, nil
}

func discoverOpenAIModels(ctx context.Context, provider Provider, timeout time.Duration) ([]Model, error) {
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := getJSON(ctx, provider.ListURL, timeout, &body); err != nil {
		return nil, err
	}
	var models []Model
	for _, item := range body.Data {
		if item.ID == "" {
			continue
		}
		models = append(models, Model{
			ProviderID: provider.ID,
			ID:         item.ID,
			Name:       item.ID,
			BaseURL:    provider.BaseURL,
			API:        "openai-responses",
		})
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("no models returned by %s", provider.ListURL)
	}
	if provider.NativeURL != "" {
		enrichLMStudioMetadata(ctx, provider, timeout, models)
	}
	return models, nil
}

func enrichLMStudioMetadata(ctx context.Context, provider Provider, timeout time.Duration, models []Model) {
	metadata, err := discoverLMStudioNativeMetadata(ctx, provider, timeout)
	if err != nil {
		return
	}
	for i := range models {
		if meta, ok := metadata[models[i].ID]; ok {
			if models[i].Metadata == nil {
				models[i].Metadata = map[string]string{}
			}
			for key, value := range meta {
				models[i].Metadata[key] = value
			}
		}
	}
}

func discoverLMStudioNativeMetadata(ctx context.Context, provider Provider, timeout time.Duration) (map[string]map[string]string, error) {
	var body struct {
		Models []struct {
			Key              string `json:"key"`
			DisplayName      string `json:"display_name"`
			Architecture     string `json:"architecture"`
			ParamsString     string `json:"params_string"`
			MaxContextLength int    `json:"max_context_length"`
			LoadedInstances  []struct {
				ID     string `json:"id"`
				Config struct {
					ContextLength int `json:"context_length"`
				} `json:"config"`
			} `json:"loaded_instances"`
		} `json:"models"`
	}
	if err := getJSON(ctx, provider.NativeURL, timeout, &body); err != nil {
		return nil, err
	}
	out := map[string]map[string]string{}
	for _, item := range body.Models {
		meta := compactMetadata(map[string]string{
			"display_name":       item.DisplayName,
			"architecture":       item.Architecture,
			"parameters":         item.ParamsString,
			"max_context_length": intString(item.MaxContextLength),
		})
		if len(item.LoadedInstances) > 0 {
			contextLength := item.LoadedInstances[0].Config.ContextLength
			for _, instance := range item.LoadedInstances {
				instanceMeta := copyStringMap(meta)
				instanceMeta["loaded_instance_id"] = instance.ID
				if instance.Config.ContextLength > 0 {
					instanceMeta["context_length"] = intString(instance.Config.ContextLength)
				}
				out[instance.ID] = compactMetadata(instanceMeta)
				if instance.ID == item.Key {
					contextLength = instance.Config.ContextLength
				}
			}
			if contextLength > 0 {
				meta["context_length"] = intString(contextLength)
			}
		}
		if item.Key != "" {
			out[item.Key] = compactMetadata(meta)
		}
	}
	return out, nil
}

func getJSON(ctx context.Context, url string, timeout time.Duration, dst any) error {
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("GET %s returned %s", url, resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return err
	}
	return nil
}

func requireOllamaAnthropicCompatibility(ctx context.Context, model Model, timeout time.Duration) error {
	var body struct {
		Version string `json:"version"`
	}
	baseURL := strings.TrimSuffix(model.BaseURL, "/v1")
	if err := getJSON(ctx, strings.TrimSuffix(baseURL, "/")+"/api/version", timeout, &body); err != nil {
		return fmt.Errorf("checking Ollama version for Claude Code: %w", err)
	}
	if !versionAtLeast(body.Version, "0.14.0") {
		return fmt.Errorf("Ollama %s does not support Claude Code's Anthropic Messages API; upgrade Ollama to 0.14.0 or newer", body.Version)
	}
	return nil
}

func versionAtLeast(got, want string) bool {
	gotParts := versionParts(got)
	wantParts := versionParts(want)
	for i := 0; i < len(wantParts); i++ {
		if gotParts[i] != wantParts[i] {
			return gotParts[i] > wantParts[i]
		}
	}
	return true
}

func versionParts(version string) []int {
	fields := strings.Split(version, ".")
	out := []int{0, 0, 0}
	for i := 0; i < len(out) && i < len(fields); i++ {
		part := fields[i]
		if idx := strings.IndexFunc(part, func(r rune) bool { return r < '0' || r > '9' }); idx >= 0 {
			part = part[:idx]
		}
		parsed, err := strconv.Atoi(part)
		if err == nil {
			out[i] = parsed
		}
	}
	return out
}

func compactMetadata(in map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range in {
		if value != "" {
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func intString(value int) string {
	if value == 0 {
		return ""
	}
	return fmt.Sprintf("%d", value)
}

func copyStringMap(in map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range in {
		out[key] = value
	}
	return out
}
