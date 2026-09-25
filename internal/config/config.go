package config

import (
	"os"

	"github.com/joho/godotenv"
)

// Config holds the application configuration.
type Config struct {
	TaostatsAPIKey string
	DefaultNetUID  int
	RedisURL       string
	SubstrateWSURL string
}

// Load lit la config depuis le .env (si présent) puis les variables d'environnement.
// Les variables shell ont priorité sur le .env.
func Load() Config {
	_ = godotenv.Load()

	substrateURL := os.Getenv("SUBSTRATE_WS_URL")
	if substrateURL == "" {
		substrateURL = "wss://entrypoint-finney.opentensor.ai"
	}

	return Config{
		TaostatsAPIKey: os.Getenv("TAOSTATS_API_KEY"),
		DefaultNetUID:  1,
		RedisURL:       os.Getenv("REDIS_URL"),
		SubstrateWSURL: substrateURL,
	}
}

// HasAPIKey retourne true si une clé Taostats est configurée.
func (c Config) HasAPIKey() bool {
	return c.TaostatsAPIKey != ""
}

// HasRedis retourne true si une URL Redis est configurée.
func (c Config) HasRedis() bool {
	return c.RedisURL != ""
}
