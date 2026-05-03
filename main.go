package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func main() {
	if err := runCLI(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "lmwire:", err)
		os.Exit(1)
	}
}

func runCLI(args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}
	switch args[0] {
	case "discover":
		return cmdDiscover(args[1:])
	case "render":
		return cmdRender(args[1:])
	case "apply":
		return cmdApply(args[1:])
	case "env":
		return cmdEnv(args[1:])
	case "run":
		return cmdRun(args[1:])
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func cmdDiscover(args []string) error {
	fs := flag.NewFlagSet("discover", flag.ContinueOnError)
	providers := fs.String("provider", "", "comma-separated providers: ollama,lmstudio")
	jsonOut := fs.Bool("json", false, "print JSON")
	timeout := fs.Duration("timeout", 2*time.Second, "discovery timeout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	models, errs := discoverModels(context.Background(), DiscoverOptions{
		Providers: splitCSV(*providers),
		Timeout:   *timeout,
	})
	if *jsonOut {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"models": models,
			"errors": errorStrings(errs),
		})
	}
	for _, model := range models {
		fmt.Printf("%s/%s\t%s\n", model.ProviderID, model.ID, model.BaseURL)
	}
	for _, err := range errs {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
	}
	if len(models) == 0 && len(errs) > 0 {
		return fmt.Errorf("no models discovered")
	}
	return nil
}

func cmdRender(args []string) error {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	targets := fs.String("target", "", "comma-separated targets: pi,codex,claude,opencode")
	providers := fs.String("provider", "", "comma-separated providers: ollama,lmstudio")
	modelRef := fs.String("model", "", "model ref provider/model-id")
	jsonOut := fs.Bool("json", false, "print JSON")
	timeout := fs.Duration("timeout", 2*time.Second, "discovery timeout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	models, errs := discoverModels(context.Background(), DiscoverOptions{
		Providers: splitCSV(*providers),
		Timeout:   *timeout,
	})
	models, err := filterModels(models, *modelRef)
	if err != nil {
		return err
	}
	patches, envs, err := renderTargets(splitCSV(*targets), models)
	if err != nil {
		return err
	}
	if *jsonOut {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"patches": patches,
			"env":     envs,
			"errors":  errorStrings(errs),
		})
	}
	for _, patch := range patches {
		fmt.Printf("### %s: %s\n%s\n", patch.TargetID, patch.Path, string(patch.After))
	}
	if len(envs) > 0 {
		fmt.Println("### env")
		printEnv(envs, "bash")
	}
	for _, err := range errs {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
	}
	return nil
}

func cmdApply(args []string) error {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	targets := fs.String("target", "", "comma-separated targets: pi,codex,claude,opencode")
	providers := fs.String("provider", "", "comma-separated providers: ollama,lmstudio")
	modelRef := fs.String("model", "", "model ref provider/model-id")
	dryRun := fs.Bool("dry-run", false, "show writes without changing files")
	backupDir := fs.String("backup-dir", "", "backup directory")
	timeout := fs.Duration("timeout", 2*time.Second, "discovery timeout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	models, errs := discoverModels(context.Background(), DiscoverOptions{
		Providers: splitCSV(*providers),
		Timeout:   *timeout,
	})
	for _, err := range errs {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
	}
	models, err := filterModels(models, *modelRef)
	if err != nil {
		return err
	}
	if len(models) == 0 {
		return fmt.Errorf("no models discovered")
	}
	patches, envs, err := renderTargets(splitCSV(*targets), models)
	if err != nil {
		return err
	}
	if *dryRun {
		printPatchSummary(patches)
		if len(envs) > 0 {
			fmt.Println("environment exports:")
			printEnv(envs, "bash")
		}
		return nil
	}
	if err := applyPatches(patches, *backupDir, false); err != nil {
		return err
	}
	if len(envs) > 0 {
		fmt.Println("claude uses environment variables; source these or use lmwire run claude:")
		printEnv(envs, "bash")
	}
	return nil
}

func cmdEnv(args []string) error {
	target := "claude"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		target = args[0]
		args = args[1:]
	}
	fs := flag.NewFlagSet("env", flag.ContinueOnError)
	providers := fs.String("provider", "", "comma-separated providers: ollama,lmstudio")
	modelRef := fs.String("model", "", "model ref provider/model-id")
	shell := fs.String("shell", "bash", "shell format: bash,fish")
	timeout := fs.Duration("timeout", 2*time.Second, "discovery timeout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	models, errs := discoverModels(context.Background(), DiscoverOptions{
		Providers: splitCSV(*providers),
		Timeout:   *timeout,
	})
	for _, err := range errs {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
	}
	models, err := filterModels(models, *modelRef)
	if err != nil {
		return err
	}
	if len(models) == 0 {
		return fmt.Errorf("no models discovered")
	}
	switch target {
	case "claude":
		printEnv(renderClaudeEnv(pickDefaultModel(models)), *shell)
	case "codex":
		model := pickDefaultModel(models)
		printEnv([]EnvVar{
			{Name: "OPENAI_API_KEY", Value: "lmwire-local"},
			{Name: "OPENAI_BASE_URL", Value: model.BaseURL},
			{Name: "CODEX_OSS_BASE_URL", Value: model.BaseURL},
		}, *shell)
	default:
		return fmt.Errorf("env target %q is not supported", target)
	}
	return nil
}

func cmdRun(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("run requires an agent: codex, claude, pi, opencode")
	}
	agent := args[0]
	rest := args[1:]
	flagArgs, trailing := splitTrailingArgs(rest)
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	providers := fs.String("provider", "", "comma-separated providers: ollama,lmstudio")
	modelRef := fs.String("model", "", "model ref provider/model-id")
	contextWindow := fs.Int("context-window", 0, "override Codex model_context_window for local models")
	timeout := fs.Duration("timeout", 2*time.Second, "discovery timeout")
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	runArgs := append([]string(nil), fs.Args()...)
	if *modelRef == "" && len(runArgs) > 0 && strings.Contains(runArgs[0], "/") {
		*modelRef = runArgs[0]
		runArgs = runArgs[1:]
	}
	trailing = append(runArgs, trailing...)
	providerIDs := splitCSV(*providers)
	if len(providerIDs) == 0 {
		if providerID := providerFromModelRef(*modelRef); providerID != "" {
			providerIDs = []string{providerID}
		}
	}
	models, errs := discoverModels(context.Background(), DiscoverOptions{
		Providers: providerIDs,
		Timeout:   *timeout,
	})
	for _, err := range errs {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
	}
	models, err := filterModels(models, *modelRef)
	if err != nil {
		return err
	}
	if len(models) == 0 {
		return fmt.Errorf("no models discovered")
	}
	model := pickDefaultModel(models)
	if err := prepareAgentRun(agent, model, *contextWindow, *timeout); err != nil {
		return err
	}
	return launchAgent(agent, model, trailing)
}

func prepareAgentRun(agent string, model Model, contextWindow int, timeout time.Duration) error {
	switch agent {
	case "claude":
		if model.ProviderID == "ollama" {
			return requireOllamaAnthropicCompatibility(context.Background(), model, timeout)
		}
		return nil
	case "pi":
		patch, err := renderPi([]Model{model})
		if err != nil {
			return err
		}
		return applyPatches([]FilePatch{patch}, "", false)
	case "codex":
		patch, err := renderCodexWithContext([]Model{model}, contextWindow)
		if err != nil {
			return err
		}
		return applyPatches([]FilePatch{patch}, "", false)
	default:
		return nil
	}
}

func defaultCodexContextWindow() int {
	value := os.Getenv("LMWIRE_CODEX_CONTEXT_WINDOW")
	if value == "" {
		return 4096
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return 4096
	}
	return parsed
}

func codexContextWindowForModel(model Model, override int) int {
	if override > 0 {
		return override
	}
	for _, key := range []string{"context_length", "max_context_length"} {
		if value := model.Metadata[key]; value != "" {
			parsed, err := strconv.Atoi(value)
			if err == nil && parsed > 0 {
				return parsed
			}
		}
	}
	return defaultCodexContextWindow()
}

func launchAgent(agent string, model Model, args []string) error {
	cmdName, cmdArgs, envVars, err := agentCommand(agent, model, args)
	if err != nil {
		return err
	}
	cmd := exec.Command(cmdName, cmdArgs...)
	cmd.Env = appendEnv(os.Environ(), envVars)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func agentCommand(agent string, model Model, args []string) (string, []string, []EnvVar, error) {
	var cmdName string
	var cmdArgs []string
	var envVars []EnvVar
	switch agent {
	case "claude":
		cmdName = "claude"
		cmdArgs = args
		if !hasFlag(args, "--model") && !hasFlag(args, "-m") {
			cmdArgs = append([]string{"--model", model.ID}, args...)
		}
		envVars = renderClaudeEnv(model)
	case "codex":
		cmdName = "codex"
		cmdArgs = append([]string{"--profile", "lmwire_" + sanitizeID(model.ProviderID+"_"+model.ID)}, args...)
	case "pi":
		cmdName = "pi"
		cmdArgs = append([]string{"--model", model.ProviderID + "/" + model.ID}, args...)
	case "opencode":
		cmdName = "opencode"
		cmdArgs = append([]string{"--model", openCodeModelRef(model)}, args...)
		envVars = append(envVars, EnvVar{Name: "OPENCODE_CONFIG_CONTENT", Value: openCodeInlineConfig(model)})
	default:
		return "", nil, nil, fmt.Errorf("unknown agent %q", agent)
	}
	return cmdName, cmdArgs, envVars, nil
}

func hasFlag(args []string, names ...string) bool {
	want := map[string]bool{}
	for _, name := range names {
		want[name] = true
	}
	for _, arg := range args {
		if want[arg] {
			return true
		}
		if idx := strings.IndexByte(arg, '='); idx > 0 && want[arg[:idx]] {
			return true
		}
	}
	return false
}

func openCodeModelRef(model Model) string {
	return openCodeProviderID(model.ProviderID) + "/" + model.ID
}

func openCodeProviderID(providerID string) string {
	return sanitizeID(providerID)
}

func openCodeInlineConfig(model Model) string {
	providerID := openCodeProviderID(model.ProviderID)
	data := map[string]any{
		"$schema": "https://opencode.ai/config.json",
		"model":   providerID + "/" + model.ID,
		"provider": map[string]any{
			providerID: map[string]any{
				"npm":     "@ai-sdk/openai-compatible",
				"name":    openCodeProviderName(model.ProviderID),
				"options": map[string]any{"baseURL": model.BaseURL},
				"models": map[string]any{
					model.ID: map[string]any{"name": model.Name},
				},
			},
		},
	}
	out, err := json.Marshal(data)
	if err != nil {
		return "{}"
	}
	return string(out)
}

func openCodeProviderName(providerID string) string {
	switch providerID {
	case "lmstudio":
		return "LM Studio (local)"
	case "ollama":
		return "Ollama (local)"
	default:
		return providerID + " (local)"
	}
}

func filterModels(models []Model, ref string) ([]Model, error) {
	if ref == "" {
		return models, nil
	}
	provider, id, ok := strings.Cut(ref, "/")
	if !ok || provider == "" || id == "" {
		return nil, fmt.Errorf("model must be provider/model-id")
	}
	var out []Model
	for _, model := range models {
		if model.ProviderID == provider && model.ID == id {
			out = append(out, model)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("model %q was not discovered", ref)
	}
	return out, nil
}

func providerFromModelRef(ref string) string {
	provider, _, ok := strings.Cut(ref, "/")
	if !ok {
		return ""
	}
	return provider
}

func splitCSV(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func splitTrailingArgs(args []string) ([]string, []string) {
	for i, arg := range args {
		if arg == "--" {
			return args[:i], args[i+1:]
		}
	}
	return args, nil
}

func printPatchSummary(patches []FilePatch) {
	for _, patch := range patches {
		if string(patch.Before) == string(patch.After) {
			fmt.Printf("unchanged %s (%s)\n", patch.Path, patch.TargetID)
			continue
		}
		fmt.Printf("would write %s (%s): %d bytes -> %d bytes\n", patch.Path, patch.TargetID, len(patch.Before), len(patch.After))
	}
}

func printEnv(envs []EnvVar, shell string) {
	for _, env := range envs {
		switch shell {
		case "fish":
			fmt.Printf("set -gx %s %s;\n", env.Name, shellQuote(env.Value))
		default:
			fmt.Printf("export %s=%s\n", env.Name, shellQuote(env.Value))
		}
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func appendEnv(base []string, vars []EnvVar) []string {
	for _, env := range vars {
		base = append(base, env.Name+"="+env.Value)
	}
	return base
}

func errorStrings(errs []error) []string {
	out := make([]string, 0, len(errs))
	for _, err := range errs {
		out = append(out, err.Error())
	}
	return out
}

func printUsage() {
	fmt.Print(`lmwire configures local AI models for agent TUIs.

Usage:
  lmwire discover [--provider ollama,lmstudio] [--json]
  lmwire render [--target pi,codex,claude,opencode] [--model provider/model]
  lmwire apply [--target pi,codex,claude,opencode] [--dry-run]
  lmwire env [claude|codex] [--model provider/model] [--shell bash|fish]
  lmwire run <codex|claude|pi|opencode> [--model provider/model] -- [agent args...]

`)
}
