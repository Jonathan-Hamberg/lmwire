# ai_config

`ai_config` is a Go CLI for wiring local model servers into agent TUIs. It discovers local Ollama and LM Studio models, renders managed config for Pi, Codex, Claude Code, and OpenCode, and can launch agents with the right local-model environment.

## Build

```bash
go build ./...
```

## Discover Models

```bash
ai_config discover
ai_config discover --provider ollama
ai_config discover --provider lmstudio --json
```

Discovery is HTTP-first:

- Ollama: `http://localhost:11434/api/tags`
- LM Studio: `http://localhost:1234/v1/models`

If Ollama HTTP discovery fails, `ai_config` falls back to `ollama list`.

## Apply Config

```bash
ai_config apply --dry-run
ai_config apply
ai_config apply --target pi,codex,opencode
```

The apply command preserves existing files and upserts `ai_config` managed entries. Existing files are backed up under `~/.ai_config/backups` before writes.

Target files:

- Pi: `~/.pi/agent/models.json`
- Codex: `~/.codex/config.toml`
- OpenCode: `$XDG_CONFIG_HOME/opencode/opencode.json` or `~/.config/opencode/opencode.json`
- Claude Code: environment variables only

## Render Without Writing

```bash
ai_config render --target pi
ai_config render --target codex --provider lmstudio
ai_config render --target claude --model ollama/qwen3.5
```

## Shell Environment

```bash
ai_config env claude --model ollama/qwen3.5
eval "$(ai_config env claude --model ollama/qwen3.5 --shell bash)"
ai_config env codex --model lmstudio/google/gemma-3n-e4b
```

Claude Code exports include `ANTHROPIC_BASE_URL`, `ANTHROPIC_API_KEY`, `ANTHROPIC_MODEL`, and custom model picker variables.

## Launch Agents

```bash
ai_config run claude --model ollama/qwen3.5 -- -p "summarize this repo"
ai_config run codex --model lmstudio/google/gemma-3n-e4b
ai_config run opencode --model lmstudio/google/gemma-3n-e4b
```

Run `ai_config apply` before launching Codex, Pi, or OpenCode so their config files contain the generated provider/model entries.

## Safety Notes

- `--dry-run` reports planned writes without changing files.
- Existing files are backed up before writes.
- Codex uses custom provider IDs such as `ai_config_ollama` instead of reserved built-in provider IDs.
- OpenCode JSONC comments are not preserved in this first implementation because the CLI uses Go's standard JSON parser.
