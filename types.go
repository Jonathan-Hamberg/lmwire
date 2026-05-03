package main

import "time"

const managedMarker = "lmwire managed"

type Model struct {
	ProviderID string            `json:"provider_id"`
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	BaseURL    string            `json:"base_url"`
	API        string            `json:"api"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type Provider struct {
	ID          string
	DisplayName string
	BaseURL     string
	ListURL     string
	NativeURL   string
	Kind        string
}

type DiscoverOptions struct {
	Providers []string
	Timeout   time.Duration
}

type FilePatch struct {
	TargetID string
	Path     string
	Before   []byte
	After    []byte
}

type EnvVar struct {
	Name  string
	Value string
}

type ApplyOptions struct {
	Targets   []string
	Providers []string
	ModelRef  string
	Timeout   time.Duration
	DryRun    bool
	JSON      bool
	BackupDir string
}

type RunOptions struct {
	Agent    string
	ModelRef string
	Args     []string
	Timeout  time.Duration
}
