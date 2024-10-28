package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Project struct {
	ID         string `json:"id"` // Control plane's UUID
	APIKey     string `json:"api_key"`
	ServerAddr string `json:"server_addr"`
}

type Config struct {
	CurrentProject string             `json:"current_project,omitempty"` // User-friendly name
	Projects       map[string]Project `json:"projects"`                  // Map of name -> Project
}

const (
	defaultServer     = "http://localhost:8005"
	defaultConfigDir  = ".orra"
	defaultConfigFile = "config.json"
	dirPerm           = 0755 // rwxr-xr-x
	filePerm          = 0666
)

func getDefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}
	return filepath.Join(home, defaultConfigDir, defaultConfigFile), nil
}

func LoadOrInit(configPath string) (*Config, string, error) {
	var err error
	// If no config path provided, use default
	if configPath == "" {
		configPath, err = getDefaultConfigPath()
		if err != nil {
			return nil, "", err
		}
	}

	// Ensure config directory exists with proper permissions
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, dirPerm); err != nil {
		return nil, "", fmt.Errorf("could not create config directory: %w", err)
	}

	config := &Config{
		Projects: make(map[string]Project),
	}

	// Try to load existing config
	if data, err := os.ReadFile(configPath); err == nil {
		if err := json.Unmarshal(data, config); err != nil {
			return nil, "", fmt.Errorf("could not parse config file: %w", err)
		}
	}

	// Save config (creates file if doesn't exist, ensures it's writable)
	if err := SaveConfig(configPath, config); err != nil {
		return nil, "", err
	}

	return config, configPath, nil
}

// GetProject returns the project configuration either by name or falls back to current project
func GetProject(config *Config, projectName string) (*Project, string, error) {
	// If project name is provided, use it
	if projectName != "" {
		if proj, exists := config.Projects[projectName]; exists {
			return &proj, projectName, nil
		}
		return nil, "", fmt.Errorf("project %s not found", projectName)
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

func SaveProject(configPath, projectName, projectID, apiKey, serverAddr string) error {
	if configPath == "" {
		var err error
		configPath, err = getDefaultConfigPath()
		if err != nil {
			return fmt.Errorf("could not determine config path: %w", err)
		}
	}

	config, _, err := LoadOrInit(configPath)
	if err != nil {
		return fmt.Errorf("could not load config: %w", err)
	}

	if serverAddr == "" {
		serverAddr = defaultServer
	}

	config.Projects[projectName] = Project{
		ID:         projectID,
		APIKey:     apiKey,
		ServerAddr: serverAddr,
	}

	// If this is the first project, make it current
	if config.CurrentProject == "" {
		config.CurrentProject = projectName
	}

	return SaveConfig(configPath, config)
}

func SaveConfig(path string, config *Config) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("could not marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, filePerm); err != nil {
		return fmt.Errorf("could not write config file: %w", err)
	}

	return nil
}
