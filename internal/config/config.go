package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server ServerConfig `yaml:"server"`
	DB     DBConfig     `yaml:"db"`
	MET    METConfig    `yaml:"met"`
	Log    LogConfig    `yaml:"log"`
}

type ServerConfig struct {
	Port               int      `yaml:"port"`
	CORSAllowedOrigins []string `yaml:"cors_allowed_origins"`
}

type DBConfig struct {
	URL string `yaml:"url"`
}

type METConfig struct {
	UserAgent string `yaml:"user_agent"`
	BaseURL   string `yaml:"base_url"`
}

type LogConfig struct {
	Level string `yaml:"level"`
}

// Load reads config from the YAML file at path, then applies environment
// variable overrides. Environment variables take precedence.
//
// Supported env vars:
//   - SERVER_PORT
//   - SERVER_CORS_ALLOWED_ORIGINS (comma-separated)
//   - DB_URL
//   - MET_USER_AGENT
//   - MET_BASE_URL
//   - LOG_LEVEL
func Load(path string) (*Config, error) {
	cfg := defaults()

	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("config: read file: %w", err)
	}

	if len(data) > 0 {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("config: parse yaml: %w", err)
		}
	}

	applyEnv(cfg)
	return cfg, nil
}

func defaults() *Config {
	return &Config{
		Server: ServerConfig{
			Port:               8080,
			CORSAllowedOrigins: []string{"http://localhost:8081", "http://127.0.0.1:8081"},
		},
		DB: DBConfig{URL: "postgres://localhost/skyn"},
		MET: METConfig{
			UserAgent: "skynapi/1.0 (met_no@jvanrhyn.co.za)",
			BaseURL:   "https://api.met.no/weatherapi/locationforecast/2.0",
		},
		Log: LogConfig{Level: "info"},
	}
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("SERVER_PORT"); v != "" {
		fmt.Sscan(v, &cfg.Server.Port)
	}
	if v := os.Getenv("SERVER_CORS_ALLOWED_ORIGINS"); v != "" {
		origins := make([]string, 0)
		for origin := range strings.SplitSeq(v, ",") {
			origin = strings.TrimSpace(origin)
			if origin == "" {
				continue
			}
			origins = append(origins, origin)
		}
		cfg.Server.CORSAllowedOrigins = origins
	}
	if v := os.Getenv("DB_URL"); v != "" {
		cfg.DB.URL = v
	}
	if v := os.Getenv("MET_USER_AGENT"); v != "" {
		cfg.MET.UserAgent = v
	}
	if v := os.Getenv("MET_BASE_URL"); v != "" {
		cfg.MET.BaseURL = v
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.Log.Level = strings.ToLower(v)
	}
}
