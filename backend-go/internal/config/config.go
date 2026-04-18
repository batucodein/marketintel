package config

import (
	"fmt"
	"strings"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	// App
	AppName     string `envconfig:"APP_NAME" default:"MarketIntel"`
	AppEnv      string `envconfig:"APP_ENV" default:"development"`
	LogLevel    string `envconfig:"LOG_LEVEL" default:"INFO"`
	Port        int    `envconfig:"PORT" default:"8000"`
	CORSOrigins string `envconfig:"CORS_ORIGINS" default:"http://localhost:3000"`

	// Database
	DatabaseURL string `envconfig:"DATABASE_URL" required:"true"`

	// Redis
	RedisURL string `envconfig:"REDIS_URL" default:"redis://localhost:6379/0"`

	// Auth
	SecretKey                string `envconfig:"SECRET_KEY" default:"change-me-in-production"`
	AccessTokenExpireMinutes int    `envconfig:"ACCESS_TOKEN_EXPIRE_MINUTES" default:"30"`
	RefreshTokenExpireDays   int    `envconfig:"REFRESH_TOKEN_EXPIRE_DAYS" default:"7"`

	// AI - Claude
	AnthropicAPIKey   string `envconfig:"ANTHROPIC_API_KEY"`
	ClaudeSonnetModel string `envconfig:"CLAUDE_SONNET_MODEL" default:"claude-sonnet-4-6"`
	ClaudeHaikuModel  string `envconfig:"CLAUDE_HAIKU_MODEL" default:"claude-haiku-4-5-20251001"`
	ClaudeOpusModel   string `envconfig:"CLAUDE_OPUS_MODEL" default:"claude-opus-4-6"`

	// AI - Azure OpenAI
	AzureOpenAIAPIKey    string `envconfig:"AZURE_OPENAI_API_KEY"`
	AzureOpenAIEndpoint  string `envconfig:"AZURE_OPENAI_ENDPOINT"`
	AzureOpenAIMiniModel string `envconfig:"AZURE_OPENAI_MINI_MODEL" default:"gpt-5.4-mini"`
	AzureOpenAINanoModel string `envconfig:"AZURE_OPENAI_NANO_MODEL" default:"gpt-5.4-nano"`

	// AI - Gemini
	GoogleAIAPIKey       string `envconfig:"GOOGLE_AI_API_KEY"`
	GeminiProModel       string `envconfig:"GEMINI_PRO_MODEL" default:"gemini-2.5-pro"`
	GeminiFlashModel     string `envconfig:"GEMINI_FLASH_MODEL" default:"gemini-2.5-flash"`
	GeminiFlashLiteModel string `envconfig:"GEMINI_FLASH_LITE_MODEL" default:"gemini-2.5-flash-lite"`

	// Google Places
	GooglePlacesAPIKey string `envconfig:"GOOGLE_PLACES_API_KEY"`

	// UN Comtrade
	ComtradeAPIKey string `envconfig:"COMTRADE_API_KEY"`

	// Tendata
	TendataAPIKey    string `envconfig:"TENDATA_API_KEY"`
	TendataAPISecret string `envconfig:"TENDATA_API_SECRET"`
	TendataBaseURL   string `envconfig:"TENDATA_BASE_URL" default:"https://open-api.tendata.cn"`

	// REST Countries
	RESTCountriesBaseURL string `envconfig:"REST_COUNTRIES_BASE_URL" default:"https://restcountries.com/v3.1"`
}

// ParsedCORSOrigins returns CORS origins as a slice.
func (c *Config) ParsedCORSOrigins() []string {
	raw := strings.Trim(c.CORSOrigins, "[]\"")
	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.Trim(p, " \"")
		if p != "" {
			origins = append(origins, p)
		}
	}
	return origins
}

// PostgresDSN converts the SQLAlchemy-style URL to a pgx-compatible DSN.
// Input:  postgresql+asyncpg://user:pass@host:port/db
// Output: postgres://user:pass@host:port/db?sslmode=disable
func (c *Config) PostgresDSN() string {
	dsn := c.DatabaseURL
	dsn = strings.Replace(dsn, "postgresql+asyncpg://", "postgres://", 1)
	dsn = strings.Replace(dsn, "postgresql://", "postgres://", 1)
	if !strings.Contains(dsn, "sslmode=") {
		if strings.Contains(dsn, "?") {
			dsn += "&sslmode=disable"
		} else {
			dsn += "?sslmode=disable"
		}
	}
	return dsn
}

// Validate checks that critical secrets are set.
// Returns an error listing all missing required config.
func (c *Config) Validate() error {
	var missing []string

	if c.SecretKey == "" || c.SecretKey == "change-me-in-production" {
		if c.AppEnv == "production" {
			missing = append(missing, "SECRET_KEY (must be changed for production)")
		}
	}

	// At least one AI provider must be configured
	hasAI := (c.AzureOpenAIAPIKey != "" && c.AzureOpenAIEndpoint != "") ||
		c.AnthropicAPIKey != "" ||
		c.GoogleAIAPIKey != ""
	if !hasAI {
		missing = append(missing, "at least one AI provider (AZURE_OPENAI_API_KEY+ENDPOINT, ANTHROPIC_API_KEY, or GOOGLE_AI_API_KEY)")
	}

	if c.GooglePlacesAPIKey == "" {
		missing = append(missing, "GOOGLE_PLACES_API_KEY")
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required config:\n  - %s", strings.Join(missing, "\n  - "))
	}
	return nil
}

func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	return &cfg, nil
}
