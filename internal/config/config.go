package config

import (
	"os"
	"path/filepath"
)

type Config struct {
	DBPath    string
	ModelPath string
	Port      string
	Project   string
}

func LoadConfig() *Config {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}

	defaultDBDir := filepath.Join(home, ".cache", "codebase-memory-mcp")
	_ = os.MkdirAll(defaultDBDir, 0755)
	defaultDBPath := filepath.Join(defaultDBDir, "graph.db")

	defaultModelDir := filepath.Join(defaultDBDir, "models")
	_ = os.MkdirAll(defaultModelDir, 0755)
	defaultModelPath := filepath.Join(defaultModelDir, "nomic-embed-text.onnx")

	cfg := &Config{
		DBPath:    getEnv("CBM_DB_PATH", defaultDBPath),
		ModelPath: getEnv("CBM_MODEL_PATH", defaultModelPath),
		Port:      getEnv("CBM_PORT", "9749"),
		Project:   getEnv("CBM_PROJECT", "default"),
	}

	return cfg
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
