package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
)

func getConfigDir() (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", err
	}
	configDir := filepath.Join(usr.HomeDir, ".config", "cbracal")
	return configDir, nil
}

func loadConfig() (*Config, error) {
	// Try current directory first (dev mode)
	localConfig := "calendars.json"
	if _, err := os.Stat(localConfig); err == nil {
		file, err := os.Open(localConfig)
		if err != nil {
			return nil, err
		}
		defer file.Close()

		var config Config
		decoder := json.NewDecoder(file)
		if err := decoder.Decode(&config); err != nil {
			return nil, err
		}

		return &config, nil
	}

	// Fall back to standard config directory (build version)
	configDir, err := getConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get config directory: %v", err)
	}

	configPath := filepath.Join(configDir, "calendars.json")
	file, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var config Config
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		return nil, err
	}

	return &config, nil
}
