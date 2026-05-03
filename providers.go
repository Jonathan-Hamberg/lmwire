package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
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
	return models, nil
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
