package config

import (
	"os"
	"github.com/joho/godotenv"
)
func Load() {
	_ = godotenv.Load()
}
func GetEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}
