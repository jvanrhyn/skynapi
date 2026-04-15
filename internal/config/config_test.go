package config_test

import (
	"os"
	"testing"

	"github.com/jvanrhyn/skynapi/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_Defaults(t *testing.T) {
	cfg, err := config.Load("nonexistent.yaml")
	require.NoError(t, err)

	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, "postgres://localhost/skyn", cfg.DB.URL)
	assert.Equal(t, "skynapi/1.0 (met_no@jvanrhyn.co.za)", cfg.MET.UserAgent)
	assert.Equal(t, "info", cfg.Log.Level)
}

func TestLoad_EnvOverrides(t *testing.T) {
	t.Setenv("SERVER_PORT", "9090")
	t.Setenv("DB_URL", "postgres://localhost/testdb")
	t.Setenv("LOG_LEVEL", "DEBUG")

	cfg, err := config.Load("nonexistent.yaml")
	require.NoError(t, err)

	assert.Equal(t, 9090, cfg.Server.Port)
	assert.Equal(t, "postgres://localhost/testdb", cfg.DB.URL)
	assert.Equal(t, "debug", cfg.Log.Level)
}

func TestLoad_YAMLFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "config*.yaml")
	require.NoError(t, err)

	_, err = f.WriteString("server:\n  port: 3000\nlog:\n  level: warn\n")
	require.NoError(t, err)
	f.Close()

	cfg, err := config.Load(f.Name())
	require.NoError(t, err)

	assert.Equal(t, 3000, cfg.Server.Port)
	assert.Equal(t, "warn", cfg.Log.Level)
	// non-overridden fields keep defaults
	assert.Equal(t, "postgres://localhost/skyn", cfg.DB.URL)
}
