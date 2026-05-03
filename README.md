# lmwire

`lmwire` is a Go CLI for wiring local model servers into agent TUIs. It discovers local Ollama and LM Studio models, renders managed config for Pi, Codex, Claude Code, OpenCode, and Microsoft Copilot, and can launch agents with the right local-model environment.

## Build

```bash
go build ./...
```

## Discover Models

```bash
lmwire discover
lmwire discover --provider ollama
lmwire discover --provider lmstudio --json
```

Discovery is HTTP-first:

- Ollama: `http://localhost:11434/api/tags`
- LM Studio: `http://localhost:1234/v1/models`

If Ollama HTTP discovery fails, `lmwire` falls back to `ollama list`.

## Apply Config

```bash
lmwire apply --dry-run
lmwire apply --dry-run --target codex --provider lmstudio
lmwire apply
lmwire apply --target pi,codex,opencode,copilot
```

The apply command preserves existing files and upserts `lmwire` managed entries. Use `--dry-run` to print the generated config without writing. Existing files are backed up under `~/.lmwire/backups` before writes.

Target files:

- Pi: `~/.pi/agent/models.json`
- Codex: `~/.codex/config.toml`
- OpenCode: `$XDG_CONFIG_HOME/opencode/opencode.json` or `~/.config/opencode/opencode.json`
- Claude Code: environment variables only; use `lmwire run claude`
- Microsoft Copilot: environment variables only; use `lmwire run copilot`

Claude Code receives `ANTHROPIC_BASE_URL`, `ANTHROPIC_AUTH_TOKEN`, `ANTHROPIC_API_KEY=""`, `ANTHROPIC_MODEL`, and custom model picker variables from `lmwire run`.
For LM Studio, `ANTHROPIC_BASE_URL` points at `http://localhost:1234` so Claude Code uses LM Studio's Anthropic-compatible `/v1/messages` endpoint.
Microsoft Copilot receives `COPILOT_PROVIDER_BASE_URL`, `COPILOT_PROVIDER_TYPE=openai`, `COPILOT_PROVIDER_API_KEY=""`, `COPILOT_PROVIDER_MODEL_ID`, and `COPILOT_PROVIDER_WIRE_MODEL` from `lmwire run`.

## Launch Agents

```bash
lmwire run claude --model ollama/qwen3.5 -- -p "summarize this repo"
lmwire run codex --model lmstudio/google/gemma-3n-e4b
lmwire run codex lmstudio/openai/gpt-oss-20b --context-window 8192
lmwire run opencode --model lmstudio/google/gemma-3n-e4b
lmwire run opencode lmstudio/google/gemma-3n-e4b
lmwire run copilot --model ollama/gpt-oss:20b
lmwire run copilot lmstudio/openai/gpt-oss-20b
```

Run `lmwire apply` before launching Codex so its config file contains the generated provider/model entries.
For Pi, `run` writes the selected model into `~/.pi/agent/models.json` before launch.
For OpenCode, `run` also passes `OPENCODE_CONFIG_CONTENT` so the selected local provider/model is available even if you have not run `apply` yet.
For Copilot, `run` starts `copilot` with GitHub Copilot CLI BYOK environment variables for the selected local provider. lmwire does not pass Copilot's `--model` flag by default; it sets a colon-free `COPILOT_PROVIDER_MODEL_ID` and sends the selected local model as `COPILOT_PROVIDER_WIRE_MODEL`.
For Codex, LM Studio models use the loaded instance context length reported by `GET /api/v1/models` when available. Use `--context-window` to override it, or `LMWIRE_CODEX_CONTEXT_WINDOW` as the final fallback.

## Safety Notes

- `apply --dry-run` prints generated config without changing files. It does not print or apply environment-only target variables.
- Existing files are backed up before writes.
- Codex uses custom provider IDs such as `lmwire_ollama` instead of reserved built-in provider IDs.
- OpenCode JSONC comments are not preserved in this first implementation because the CLI uses Go's standard JSON parser.
