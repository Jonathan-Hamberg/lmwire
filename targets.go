package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func renderTargets(targets []string, models []Model) ([]FilePatch, []EnvVar, error) {
	if len(targets) == 0 {
		targets = []string{"pi", "codex", "claude", "opencode"}
	}
	var patches []FilePatch
	var envs []EnvVar
	for _, target := range targets {
		switch strings.ToLower(strings.TrimSpace(target)) {
		case "pi":
			patch, err := renderPi(models)
			if err != nil {
				return nil, nil, err
			}
			patches = append(patches, patch)
		case "codex":
			patch, err := renderCodex(models)
			if err != nil {
				return nil, nil, err
			}
			patches = append(patches, patch)
		case "claude":
			envs = append(envs, renderClaudeEnv(pickDefaultModel(models))...)
		case "opencode":
			patch, err := renderOpenCode(models)
			if err != nil {
				return nil, nil, err
			}
			patches = append(patches, patch)
		case "":
		default:
			return nil, nil, fmt.Errorf("unknown target %q", target)
		}
	}
	return patches, envs, nil
}

func renderPi(models []Model) (FilePatch, error) {
	path := defaultConfigPath("pi")
	before := readExisting(path)

	var cfg map[string]any
	if len(bytes.TrimSpace(before)) == 0 {
		cfg = map[string]any{}
	} else if err := json.Unmarshal(before, &cfg); err != nil {
		return FilePatch{}, fmt.Errorf("read %s: %w", path, err)
	}
	providers := objectMap(cfg["providers"])
	cfg["providers"] = providers

	grouped := groupModels(models)
	for providerID, providerModels := range grouped {
		if len(providerModels) == 0 {
			continue
		}
		provider := map[string]any{
			"baseUrl": providerModels[0].BaseURL,
			"api":     "openai-completions",
			"apiKey":  providerID,
			"compat": map[string]any{
				"supportsDeveloperRole":   false,
				"supportsReasoningEffort": false,
			},
			"models": piModelList(providerModels),
			"x-lmwire": map[string]any{
				"managed": true,
			},
		}
		providers[providerID] = provider
	}
	after, err := marshalJSON(cfg)
	if err != nil {
		return FilePatch{}, err
	}
	return FilePatch{TargetID: "pi", Path: path, Before: before, After: after}, nil
}

func piModelList(models []Model) []map[string]any {
	sortModels(models)
	out := make([]map[string]any, 0, len(models))
	for _, model := range models {
		out = append(out, map[string]any{
			"id":   model.ID,
			"name": model.Name,
			"cost": map[string]any{
				"input":      0,
				"output":     0,
				"cacheRead":  0,
				"cacheWrite": 0,
			},
		})
	}
	return out
}

func renderCodex(models []Model) (FilePatch, error) {
	return renderCodexWithContext(models, defaultCodexContextWindow())
}

func renderCodexWithContext(models []Model, contextWindow int) (FilePatch, error) {
	path := defaultConfigPath("codex")
	before := readExisting(path)
	base := stripManagedTomlBlock(string(before))
	var buf bytes.Buffer
	buf.WriteString(strings.TrimRight(base, "\n"))
	if buf.Len() > 0 {
		buf.WriteString("\n\n")
	}
	buf.WriteString("# ")
	buf.WriteString(managedMarker)
	buf.WriteString(" begin\n")
	grouped := groupModels(models)
	ids := sortedKeys(grouped)
	for _, providerID := range ids {
		providerModels := grouped[providerID]
		if len(providerModels) == 0 {
			continue
		}
		codexProviderID := "lmwire_" + sanitizeID(providerID)
		fmt.Fprintf(&buf, "\n[model_providers.%s]\n", codexProviderID)
		fmt.Fprintf(&buf, "name = %q\n", "lmwire "+providerID)
		fmt.Fprintf(&buf, "base_url = %q\n", providerModels[0].BaseURL)
		fmt.Fprintf(&buf, "requires_openai_auth = false\n")
		for _, model := range sortedModelCopy(providerModels) {
			profile := "lmwire_" + sanitizeID(providerID+"_"+model.ID)
			fmt.Fprintf(&buf, "\n[profiles.%s]\n", profile)
			fmt.Fprintf(&buf, "model_provider = %q\n", codexProviderID)
			fmt.Fprintf(&buf, "model = %q\n", model.ID)
			if window := codexContextWindowForModel(model, contextWindow); window > 0 {
				fmt.Fprintf(&buf, "model_context_window = %d\n", window)
			}
		}
	}
	buf.WriteString("\n# ")
	buf.WriteString(managedMarker)
	buf.WriteString(" end\n")
	return FilePatch{TargetID: "codex", Path: path, Before: before, After: buf.Bytes()}, nil
}

func renderClaudeEnv(model Model) []EnvVar {
	if model.ID == "" {
		return nil
	}
	baseURL := strings.TrimSuffix(model.BaseURL, "/v1")
	authToken := model.ProviderID
	if authToken == "" {
		authToken = "lmwire-local"
	}
	return []EnvVar{
		{Name: "ANTHROPIC_BASE_URL", Value: baseURL},
		{Name: "ANTHROPIC_AUTH_TOKEN", Value: authToken},
		{Name: "ANTHROPIC_API_KEY", Value: ""},
		{Name: "ANTHROPIC_MODEL", Value: model.ID},
		{Name: "ANTHROPIC_CUSTOM_MODEL_OPTION", Value: model.ID},
		{Name: "ANTHROPIC_CUSTOM_MODEL_OPTION_NAME", Value: model.Name},
		{Name: "ANTHROPIC_CUSTOM_MODEL_OPTION_DESCRIPTION", Value: model.ProviderID + " local model"},
	}
}

func renderOpenCode(models []Model) (FilePatch, error) {
	path := defaultConfigPath("opencode")
	before := readExisting(path)
	var cfg map[string]any
	if len(bytes.TrimSpace(before)) == 0 {
		cfg = map[string]any{"$schema": "https://opencode.ai/config.json"}
	} else if err := json.Unmarshal(before, &cfg); err != nil {
		return FilePatch{}, fmt.Errorf("read %s: %w", path, err)
	}
	providers := objectMap(cfg["provider"])
	cfg["provider"] = providers

	grouped := groupModels(models)
	for providerID, providerModels := range grouped {
		if len(providerModels) == 0 {
			continue
		}
		opencodeProviderID := openCodeProviderID(providerID)
		modelMap := map[string]any{}
		for _, model := range sortedModelCopy(providerModels) {
			modelMap[model.ID] = map[string]any{
				"name": model.Name,
				"options": map[string]any{
					"temperature": 0,
				},
			}
		}
		providers[opencodeProviderID] = map[string]any{
			"name":    openCodeProviderName(providerID),
			"npm":     "@ai-sdk/openai-compatible",
			"options": map[string]any{"baseURL": providerModels[0].BaseURL},
			"models":  modelMap,
		}
		if cfg["model"] == nil && len(providerModels) > 0 {
			cfg["model"] = opencodeProviderID + "/" + providerModels[0].ID
		}
	}
	after, err := marshalJSON(cfg)
	if err != nil {
		return FilePatch{}, err
	}
	return FilePatch{TargetID: "opencode", Path: path, Before: before, After: after}, nil
}

func applyPatches(patches []FilePatch, backupDir string, dryRun bool) error {
	for _, patch := range patches {
		if bytes.Equal(patch.Before, patch.After) {
			continue
		}
		if dryRun {
			fmt.Printf("would write %s (%s)\n", patch.Path, patch.TargetID)
			continue
		}
		if err := writePatch(patch, backupDir); err != nil {
			return err
		}
		fmt.Printf("wrote %s (%s)\n", patch.Path, patch.TargetID)
	}
	return nil
}

func writePatch(patch FilePatch, backupDir string) error {
	if len(patch.Before) > 0 {
		if err := backupFile(patch.Path, patch.Before, backupDir); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(patch.Path), 0o755); err != nil {
		return err
	}
	tmp := patch.Path + ".tmp"
	if err := os.WriteFile(tmp, patch.After, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, patch.Path)
}

func backupFile(path string, data []byte, backupDir string) error {
	if backupDir == "" {
		backupDir = expandPath("~/.lmwire/backups")
	}
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return err
	}
	name := strings.TrimPrefix(path, string(filepath.Separator))
	name = strings.ReplaceAll(name, string(filepath.Separator), "__")
	stamp := time.Now().Format("20060102T150405")
	return os.WriteFile(filepath.Join(backupDir, name+"."+stamp+".bak"), data, 0o644)
}

func readExisting(path string) []byte {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return data
}

func marshalJSON(v any) ([]byte, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func objectMap(v any) map[string]any {
	if existing, ok := v.(map[string]any); ok {
		return existing
	}
	return map[string]any{}
}

func stripManagedTomlBlock(in string) string {
	begin := "# " + managedMarker + " begin"
	end := "# " + managedMarker + " end"
	start := strings.Index(in, begin)
	if start == -1 {
		return in
	}
	stop := strings.Index(in[start:], end)
	if stop == -1 {
		return in[:start]
	}
	stop += start + len(end)
	return strings.TrimRight(in[:start]+in[stop:], "\n")
}

func groupModels(models []Model) map[string][]Model {
	grouped := map[string][]Model{}
	for _, model := range models {
		grouped[model.ProviderID] = append(grouped[model.ProviderID], model)
	}
	return grouped
}

func pickDefaultModel(models []Model) Model {
	if len(models) == 0 {
		return Model{}
	}
	sortModels(models)
	return models[0]
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedModelCopy(models []Model) []Model {
	out := append([]Model(nil), models...)
	sortModels(out)
	return out
}

func sortModels(models []Model) {
	sort.Slice(models, func(i, j int) bool {
		if models[i].ProviderID == models[j].ProviderID {
			return models[i].ID < models[j].ID
		}
		return models[i].ProviderID < models[j].ProviderID
	})
}

func sanitizeID(id string) string {
	id = strings.ToLower(id)
	var b strings.Builder
	lastUnderscore := false
	for _, r := range id {
		ok := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if ok {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}
