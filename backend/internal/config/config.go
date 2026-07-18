// Package config reads runtime configuration from environment variables.
package config

import (
	"os"
	"time"
)

// Config holds all runtime configuration for the server.
type Config struct {
	HTTPAddr           string
	OBSAddr            string
	OBSPassword        string
	MediaMTXAPIURL     string
	MediaSourceBaseURL string
	APIToken           string
	SyncInterval       time.Duration
	ProgramScene       string
	LogLevel           string
}

// Load reads configuration from environment variables, applying defaults for
// any variable that is unset.
func Load() Config {
	return Config{
		HTTPAddr:           getEnv("HTTP_ADDR", ":8080"),
		OBSAddr:            getEnv("OBS_ADDR", "localhost:4455"),
		OBSPassword:        getEnv("OBS_PASSWORD", ""),
		MediaMTXAPIURL:     getEnv("MEDIAMTX_API_URL", "http://localhost:9997"),
		MediaSourceBaseURL: getEnv("MEDIA_SOURCE_BASE_URL", "rtmp://localhost:1935"),
		APIToken:           getEnv("API_TOKEN", "dev-token"),
		SyncInterval:       getEnvDuration("SYNC_INTERVAL", 3*time.Second),
		ProgramScene:       getEnv("PROGRAM_SCENE", "Program"),
		LogLevel:           getEnv("LOG_LEVEL", "info"),
	}
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}
