package config

import (
	"encoding/json"
	"os"
)

type Config struct {
	Port               string   `json:"port"`
	AllowedExtensions  []string `json:"allowed_extensions"`
	MaxFilesPerTask    int      `json:"max_files_per_task"`
	MaxConcurrentTasks int      `json:"max_concurrent_tasks"`
}

func LoadConfig(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	cfg := &Config{}
	decoder := json.NewDecoder(file)
	err = decoder.Decode(cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}
