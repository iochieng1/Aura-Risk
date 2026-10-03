package config

import (
	"log"
	"os"
	"strconv"
	"time"

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

func GetBool(key string, fallback bool) bool {
	value, exists := os.LookupEnv(key)
	if !exists {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		log.Printf("Invalid boolean for %s=%q, using %v", key, value, fallback)
		return fallback
	}
	return parsed
}

func GetDuration(key string, fallback time.Duration) time.Duration {
	value, exists := os.LookupEnv(key)
	if !exists {
		return fallback
	}
	if value == "0" {
		return 0
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		log.Printf("Invalid duration for %s=%q, using %v", key, value, fallback)
		return fallback
	}
	return parsed
}
