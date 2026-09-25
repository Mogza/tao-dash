package config

import (
	"os"

	"github.com/joho/godotenv"
)

// Config holds the application configuration.
type Config struct {
	TaostatsAPIKey string
	DefaultNetUID  int
}

// Load reads configuration from a .env file (if present) then from environment variables.
// Environment variables set in the shell take precedence over the .env file.
// TAOSTATS_API_KEY is required for live metagraph data.
func Load() Config {
	// Silently ignore if .env is absent — not an error in production.
	_ = godotenv.Load()

	return Config{
		TaostatsAPIKey: os.Getenv("TAOSTATS_API_KEY"),
		DefaultNetUID:  1,
	}
}

// HasAPIKey returns true if a Taostats API key is configured.
func (c Config) HasAPIKey() bool {
	return c.TaostatsAPIKey != ""
}
