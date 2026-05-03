  # ai_config Go CLI Design

  ## Summary

  Build ai_config, a Go CLI that discovers local model servers and writes managed config for Pi, Codex, Claude Code, and OpenCode so local Ollama and LM Studio models are selectable and
  launchable from agent TUIs.

  Chosen v1 defaults:

  - Use HTTP discovery first.
  - Preserve user config and upsert only ai_config-managed entries.
  - Create backups before every write.
  - Support all four targets: Pi, Codex, Claude Code, OpenCode.

  Docs referenced:

  - Pi custom models: https://pi.dev/docs/latest/models
  - Codex config reference: https://developers.openai.com/codex/config-reference
  - Claude Code env vars: https://code.claude.com/docs/en/env-vars
  - Ollama Claude Code integration: https://docs.ollama.com/integrations/claude-code
  - OpenCode models: https://opencode.ai/docs/models/
  - Ollama list models: https://docs.ollama.com/api/tags
  - LM Studio OpenAI-compatible models: https://lmstudio.ai/docs/developer/openai-compat/models

  ## CLI Skeleton

  # Discover local providers and models
  ai_config discover
  ai_config discover --provider ollama
  ai_config discover --provider lmstudio
  ai_config discover --json

  # Write managed config to all supported agents
  ai_config apply
  ai_config apply --target pi,codex,claude,opencode
  ai_config apply --provider ollama,lmstudio
  ai_config apply --dry-run
  ai_config apply --backup-dir ~/.ai_config/backups

  # Show generated config without writing
  ai_config render --target pi
  ai_config render --target codex --provider lmstudio
  ai_config render --target claude --model qwen2.5-coder:7b
  ai_config render --format json

  # Launch an agent with local-model environment/config
  ai_config run codex --model ollama/qwen2.5-coder:7b
  ai_config run claude --model ollama/qwen3.5
  ai_config run pi --model lmstudio/qwen/qwen3-coder
  ai_config run opencode --model lmstudio/google/gemma-3n-e4b -- --help

  # Emit shell exports for sourcing
  ai_config env claude --model ollama/qwen3.5
  ai_config env codex --model lmstudio/qwen/qwen3-coder
  eval "$(ai_config env claude --model ollama/qwen3.5 --shell bash)"
  ai_config env --shell fish

  Global flags:

  --config ~/.config/ai_config/config.toml
  --provider ollama,lmstudio
  --target pi,codex,claude,opencode
  --model <provider>/<model-id>
  --dry-run
  --json
  --verbose
  --timeout 2s

  ## Key Design

  Use a small adapter architecture:

  type Provider interface {
      ID() string
      Discover(ctx context.Context, opts DiscoverOptions) ([]Model, error)
      Endpoint() Endpoint
  }

  type Target interface {
      ID() string
      Detect(ctx context.Context) TargetState
      Render(plan ApplyPlan) ([]FilePatch, []EnvVar, error)
      Launch(ctx context.Context, args LaunchArgs) error
  }

  Core model shape:

  type Model struct {
      ProviderID string
      ID         string
      Name       string
      BaseURL    string
      API        string // openai-chat, openai-responses, anthropic
      Metadata   map[string]string
  }

  Provider discovery:

  - Ollama:
      - Probe http://localhost:11434/api/tags.
      - Convert to OpenAI-compatible base URL http://localhost:11434/v1.
      - Optional fallback: ollama list.
  - LM Studio:
      - Probe http://localhost:1234/v1/models.
      - Treat returned IDs as OpenAI-compatible model IDs.
      - Optional future native API support via /api/v1/models.

  Config application:

  - Read existing config into typed or syntax-preserving structures where practical.
  - Write only managed provider/model entries tagged with stable metadata where the target format allows.
  - Create timestamped backups before writing.
  - --dry-run prints file diffs and env exports.

  ## Target Behavior

  Pi:

  - Write ~/.pi/agent/models.json.
  - Add ollama and lmstudio providers with OpenAI-compatible APIs.
  - Use api: "openai-completions" for broad local compatibility.
  - Set compat.supportsDeveloperRole=false and compat.supportsReasoningEffort=false by default for local servers.

  Codex:

  - Write ~/.codex/config.toml.
  - Avoid overriding reserved built-in provider IDs ollama and lmstudio; use custom IDs like ai_config_ollama and ai_config_lmstudio.
  - Add profiles per discovered model:
      - ai_config_ollama_qwen2_5_coder_7b
      - ai_config_lmstudio_google_gemma_3n_e4b
  - Use custom provider base_url and default Responses API behavior where compatible.

  Claude Code:

  - Prefer env-based integration instead of config-file mutation.
  - ai_config env claude emits:
      - ANTHROPIC_BASE_URL
      - ANTHROPIC_API_KEY
      - ANTHROPIC_MODEL
      - ANTHROPIC_CUSTOM_MODEL_OPTION
      - optional display name/description vars
  - For Ollama, support launching through ollama launch claude when available, but keep direct env export as the generic path.

  OpenCode:

  - Write user OpenCode config JSON/JSONC with provider entries and model IDs.
  - Use model references as provider_id/model_id.
  - Preserve existing provider config and only upsert ai_config providers/models.

  ## Implementation Plan

  1. Scaffold Go module with Cobra CLI, internal packages for provider, target, configio, and cmd.
  2. Implement discovery:
      - HTTP client with short timeout.
      - Ollama /api/tags.
      - LM Studio /v1/models.
      - normalized model list output.
  3. Implement render/apply engine:
      - load existing files,
      - generate target-specific desired config,
      - diff,
      - backup,
      - atomic write.
  4. Implement target adapters for Pi, Codex, Claude Code env, and OpenCode.
  5. Implement run:
      - resolve model,
      - prepare env/config,
      - exec target binary with trailing args after --.
  6. Add docs:
      - README quickstart,
      - supported targets/providers matrix,
      - shell sourcing examples,
      - safety/backup behavior.

  ## Test Plan

  - Unit tests for Ollama and LM Studio discovery using httptest.
  - Golden-file render tests for Pi JSON, Codex TOML, Claude env output, and OpenCode JSON.
  - Merge tests proving user-owned config fields survive apply.
  - Dry-run tests proving no files are written.
  - Backup/atomic-write tests.
  - CLI tests for command parsing and --model provider/model-id resolution.

  ## Assumptions

  - v1 is a local developer tool, not a daemon.
  - Linux/macOS are primary; Windows path support can be added after core behavior works.
  - Managed merge is safer than overwriting full agent configs.
  - HTTP discovery is preferred because both Ollama and LM Studio expose model-list endpoints.
  - Local model metadata will be incomplete; v1 will not infer tool-calling or reasoning quality beyond conservative compatibility defaults.

