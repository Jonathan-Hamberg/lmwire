package main

import (
	"os"
	"path/filepath"
	"strings"
)

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func defaultConfigPath(target string) string {
	switch target {
	case "pi":
		return expandPath("~/.pi/agent/models.json")
	case "codex":
		return expandPath("~/.codex/config.toml")
	case "opencode":
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			return filepath.Join(xdg, "opencode", "opencode.json")
		}
		return expandPath("~/.config/opencode/opencode.json")
	default:
		return ""
	}
}
