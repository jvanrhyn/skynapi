package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	DB        DBConfig        `yaml:"db"`
	MET       METConfig       `yaml:"met"`
	Nominatim NominatimConfig `yaml:"nominatim"`
	Log       LogConfig       `yaml:"log"`
}

type ServerConfig struct {
	Port               int      `yaml:"port"`
	CORSAllowedOrigins []string `yaml:"cors_allowed_origins"`
	// RateLimitPerMinute caps requests per client IP per minute.
	// A value <= 0 disables rate limiting.
	RateLimitPerMinute int `yaml:"rate_limit_per_minute"`
	// TrustedProxyCount is how many reverse proxies sit in front of the API.
	// The shipped Compose stack puts Caddy in front, so the default is 1.
	// Set it to 0 when the API is exposed directly, otherwise clients can
	// forge X-Forwarded-For and mint themselves a fresh rate-limit bucket.
	TrustedProxyCount int `yaml:"trusted_proxy_count"`
}

type DBConfig struct {
	URL string `yaml:"url"`
}

type METConfig struct {
	UserAgent string `yaml:"user_agent"`
	BaseURL   string `yaml:"base_url"`
}

// NominatimConfig configures the reverse-geocoding upstream. Requests are
// proxied through the API rather than made from the browser so they carry the
// identifying User-Agent Nominatim's usage policy requires, and so a user's
// coordinates are not handed to a third party by the page itself.
type NominatimConfig struct {
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
//   - SERVER_RATE_LIMIT_PER_MINUTE
//   - SERVER_TRUSTED_PROXY_COUNT
//   - DB_URL
//   - MET_USER_AGENT
//   - MET_BASE_URL
//   - NOMINATIM_USER_AGENT
//   - NOMINATIM_BASE_URL
//   - LOG_LEVEL
//
// A malformed numeric override is an error rather than a silent fallback: a
// typo in a deployment variable should fail the boot, not quietly serve with
// different settings than the operator asked for.
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

	if err := applyEnv(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func defaults() *Config {
	return &Config{
		Server: ServerConfig{
			Port:               8080,
			CORSAllowedOrigins: []string{"http://localhost:8081", "http://127.0.0.1:8081"},
			RateLimitPerMinute: 120,
			TrustedProxyCount:  1,
		},
		DB: DBConfig{URL: "postgres://localhost/skyn"},
		MET: METConfig{
			UserAgent: "skynapi/1.0 (met_no@jvanrhyn.co.za)",
			BaseURL:   "https://api.met.no/weatherapi/locationforecast/2.0",
		},
		Nominatim: NominatimConfig{
			UserAgent: "skynapi/1.0 (met_no@jvanrhyn.co.za)",
			BaseURL:   "https://nominatim.openstreetmap.org",
		},
		Log: LogConfig{Level: "info"},
	}
}

func applyEnv(cfg *Config) error {
	if err := envInt("SERVER_PORT", &cfg.Server.Port); err != nil {
		return err
	}
	if err := envInt("SERVER_RATE_LIMIT_PER_MINUTE", &cfg.Server.RateLimitPerMinute); err != nil {
		return err
	}
	if err := envInt("SERVER_TRUSTED_PROXY_COUNT", &cfg.Server.TrustedProxyCount); err != nil {
		return err
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
	envString("DB_URL", &cfg.DB.URL)
	envString("MET_USER_AGENT", &cfg.MET.UserAgent)
	envString("MET_BASE_URL", &cfg.MET.BaseURL)
	envString("NOMINATIM_USER_AGENT", &cfg.Nominatim.UserAgent)
	envString("NOMINATIM_BASE_URL", &cfg.Nominatim.BaseURL)
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.Log.Level = strings.ToLower(strings.TrimSpace(v))
	}
	return nil
}

func envString(key string, dst *string) {
	if v := os.Getenv(key); v != "" {
		*dst = v
	}
}

func envInt(key string, dst *int) error {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fmt.Errorf("config: %s must be an integer, got %q", key, v)
	}
	*dst = n
	return nil
}
