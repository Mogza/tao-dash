package config

import (
	"os"
)

// Config holds the application configuration.
type Config struct {
	TaostatsAPIKey string
	DefaultNetUID  int
}

// Load reads configuration from environment variables.
// TAOSTATS_API_KEY is required for metagraph data.
// TAO_DASH_NETUID defaults to 1 (Subnet 1).
func Load() Config {
	netUID := 1

	return Config{
		TaostatsAPIKey: os.Getenv("TAOSTATS_API_KEY"),
		DefaultNetUID:  netUID,
	}
}

// HasAPIKey returns true if a Taostats API key is configured.
func (c Config) HasAPIKey() bool {
	return c.TaostatsAPIKey != ""
}
