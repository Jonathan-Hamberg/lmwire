package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func main() {
	if err := runCLI(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "ai_config:", err)
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
		fmt.Println("claude uses environment variables; source these or use ai_config run claude:")
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
			{Name: "OPENAI_API_KEY", Value: "ai_config-local"},
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
	timeout := fs.Duration("timeout", 2*time.Second, "discovery timeout")
	if err := fs.Parse(flagArgs); err != nil {
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
	return launchAgent(agent, pickDefaultModel(models), trailing)
}

func launchAgent(agent string, model Model, args []string) error {
	var cmdName string
	var cmdArgs []string
	env := os.Environ()
	switch agent {
	case "claude":
		cmdName = "claude"
		env = appendEnv(env, renderClaudeEnv(model))
	case "codex":
		cmdName = "codex"
		cmdArgs = append([]string{"--profile", "ai_config_" + sanitizeID(model.ProviderID+"_"+model.ID)}, args...)
	case "pi":
		cmdName = "pi"
		cmdArgs = append([]string{"--model", model.ProviderID + "/" + model.ID}, args...)
	case "opencode":
		cmdName = "opencode"
		cmdArgs = append([]string{"--model", "ai_config_" + sanitizeID(model.ProviderID) + "/" + model.ID}, args...)
	default:
		return fmt.Errorf("unknown agent %q", agent)
	}
	if cmdName == "claude" {
		cmdArgs = args
	}
	cmd := exec.Command(cmdName, cmdArgs...)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
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
	fmt.Print(`ai_config configures local AI models for agent TUIs.

Usage:
  ai_config discover [--provider ollama,lmstudio] [--json]
  ai_config render [--target pi,codex,claude,opencode] [--model provider/model]
  ai_config apply [--target pi,codex,claude,opencode] [--dry-run]
  ai_config env [claude|codex] [--model provider/model] [--shell bash|fish]
  ai_config run <codex|claude|pi|opencode> [--model provider/model] -- [agent args...]

`)
}
