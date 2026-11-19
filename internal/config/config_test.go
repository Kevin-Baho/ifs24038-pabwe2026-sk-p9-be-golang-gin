package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

var envKeys = []string{
	"APP_PORT", "APP_ENV", "APP_KEY", "ADMIN_USERNAME", "ADMIN_PASSWORD",
	"DB_CONNECTION", "DB_HOST", "DB_PORT", "DB_DATABASE", "DB_USERNAME",
	"DB_PASSWORD", "DB_SSLMODE", "SEARCH_ENGINE_ENDPOINT",
	"OPENAI_ENDPOINT", "OPENAI_KEY", "OPENAI_MODEL", "AI_TIMEOUT",
}

func clearEnv() {
	for _, k := range envKeys {
		_ = os.Unsetenv(k)
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	clearEnv()

	cfg := LoadConfig("")
	assert.Equal(t, "8081", cfg.AppPort)
	assert.Equal(t, "development", cfg.AppEnv)
	assert.Equal(t, "delcom_blog_secret_key_super_secure_2026_random_string_32chars", cfg.AppKey)
	assert.Equal(t, "admin", cfg.AdminUsername)
	assert.Equal(t, "admin", cfg.AdminPassword)
	assert.Equal(t, "pgsql", cfg.DBConnection)
	assert.Equal(t, "127.0.0.1", cfg.DBHost)
	assert.Equal(t, "5432", cfg.DBPort)
	assert.Equal(t, "pabwe_p9_blog", cfg.DBDatabase)
	assert.Equal(t, "postgres", cfg.DBUsername)
	assert.Equal(t, "postgres", cfg.DBPassword)
	assert.Equal(t, "disable", cfg.DBSSLMode)
	assert.Equal(t, "https://delcom.org/api/v1/search", cfg.SearchEngineEndpoint)
	assert.Equal(t, "https://delcom.org/api/v1", cfg.OpenAIEndpoint)
	assert.Equal(t, "key", cfg.OpenAIKey)
	assert.Equal(t, "nama-model", cfg.OpenAIModel)
	assert.Equal(t, 50, cfg.AITimeout)

	expectedDSN := "host=127.0.0.1 user=postgres password=postgres dbname=pabwe_p9_blog port=5432 sslmode=disable TimeZone=Asia/Jakarta"
	assert.Equal(t, expectedDSN, cfg.DSN())
}

func TestLoadConfig_CustomEnv(t *testing.T) {
	clearEnv()
	t.Cleanup(clearEnv)

	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env.test")
	content := `
APP_PORT=9000
APP_ENV=production
APP_KEY=custom-key
ADMIN_USERNAME=custom_admin
ADMIN_PASSWORD=custom_pass_admin
DB_CONNECTION=pgsql
DB_HOST=localhost
DB_PORT=5433
DB_DATABASE=custom_db
DB_USERNAME=custom_user
DB_PASSWORD=custom_pass
DB_SSLMODE=require
SEARCH_ENGINE_ENDPOINT=https://custom.api/v1/search
OPENAI_ENDPOINT=https://custom.api/v1
OPENAI_KEY=my-secret-key
OPENAI_MODEL=gpt-4o
AI_TIMEOUT=60
`
	err := os.WriteFile(envFile, []byte(content), 0644)
	assert.NoError(t, err)

	cfg := LoadConfig(envFile)
	assert.Equal(t, "9000", cfg.AppPort)
	assert.Equal(t, "production", cfg.AppEnv)
	assert.Equal(t, "custom-key", cfg.AppKey)
	assert.Equal(t, "custom_admin", cfg.AdminUsername)
	assert.Equal(t, "custom_pass_admin", cfg.AdminPassword)
	assert.Equal(t, "pgsql", cfg.DBConnection)
	assert.Equal(t, "localhost", cfg.DBHost)
	assert.Equal(t, "5433", cfg.DBPort)
	assert.Equal(t, "custom_db", cfg.DBDatabase)
	assert.Equal(t, "custom_user", cfg.DBUsername)
	assert.Equal(t, "custom_pass", cfg.DBPassword)
	assert.Equal(t, "require", cfg.DBSSLMode)
	assert.Equal(t, "https://custom.api/v1/search", cfg.SearchEngineEndpoint)
	assert.Equal(t, "https://custom.api/v1", cfg.OpenAIEndpoint)
	assert.Equal(t, "my-secret-key", cfg.OpenAIKey)
	assert.Equal(t, "gpt-4o", cfg.OpenAIModel)
	assert.Equal(t, 60, cfg.AITimeout)
}

func TestLoadConfig_InvalidAITimeout(t *testing.T) {
	clearEnv()
	t.Cleanup(clearEnv)

	_ = os.Setenv("AI_TIMEOUT", "invalid-int")
	cfg := LoadConfig("")
	assert.Equal(t, 50, cfg.AITimeout)
}
