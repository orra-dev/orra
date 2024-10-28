package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Project struct {
	APIKey     string `json:"api_key"`
	ServerAddr string `json:"server_addr"`
}

type Config struct {
	CurrentProject string             `json:"current_project,omitempty"`
	Projects       map[string]Project `json:"projects"`
}

const (
	defaultServer     = "http://localhost:8005"
	defaultConfigDir  = ".orra"
	defaultConfigFile = "config.json"
)

// LoadOrInit loads the config file if it exists, or creates a new one if it doesn't.
func LoadOrInit(configPath string) (*Config, error) {
	if configPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("could not determine home directory: %w", err)
		}
		configPath = filepath.Join(home, defaultConfigDir, defaultConfigFile)
	}

	// Ensure config directory exists
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("could not create config directory: %w", err)
	}

	config := &Config{
		Projects: make(map[string]Project),
	}

	// Try to load existing config
	if data, err := os.ReadFile(configPath); err == nil {
		if err := json.Unmarshal(data, config); err != nil {
			return nil, fmt.Errorf("could not parse config file: %w", err)
		}
	}

	// Save config (creates file if doesn't exist, ensures it's writable)
	if err := SaveConfig(configPath, config); err != nil {
		return nil, err
	}

	return config, nil
}

// GetProject returns the project configuration either by ID or falls back to current project
func GetProject(config *Config, projectID string) (*Project, string, error) {
	// If project ID is provided, use it
	if projectID != "" {
		if proj, exists := config.Projects[projectID]; exists {
			return &proj, projectID, nil
		}
		return nil, "", fmt.Errorf("project %s not found", projectID)
	}

	// Fall back to current project
	if config.CurrentProject == "" {
		return nil, "", fmt.Errorf("no project specified and no current project set")
	}

	if proj, exists := config.Projects[config.CurrentProject]; exists {
		return &proj, config.CurrentProject, nil
	}

	return nil, "", fmt.Errorf("current project %s not found", config.CurrentProject)
}

// SaveProject saves or updates a project in the config
func SaveProject(configPath string, projectID, apiKey, serverAddr string) error {
	config, err := LoadOrInit(configPath)
	if err != nil {
		return fmt.Errorf("could not load config: %w", err)
	}

	if serverAddr == "" {
		serverAddr = defaultServer
	}

	config.Projects[projectID] = Project{
		APIKey:     apiKey,
		ServerAddr: serverAddr,
	}

	// If this is the first project, make it current
	if config.CurrentProject == "" {
		config.CurrentProject = projectID
	}

	return SaveConfig(configPath, config)
}

// SaveConfig saves the configuration to disk
func SaveConfig(path string, config *Config) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("could not marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("could not write config file: %w", err)
	}

	return nil
}
