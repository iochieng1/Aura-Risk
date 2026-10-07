package config

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

const DefaultNotifierInterval = 15 * time.Minute

// devSecretValues are credentials used by the local Compose setup or shipped
// as placeholders in .env.example. Production refuses to start with them.
var devSecretValues = map[string]bool{
	"password":            true,
	"postgres":            true,
	"aurarisk":            true,
	"aurarisk-dev-secret": true,
}

// Load reads .env files, resolves secret references, and validates the
// resulting configuration. Callers should treat an error as fatal.
func Load(ctx context.Context) error {
	// Support running from aurarisk-backend/ with the .env at the repo root.
	// godotenv never overrides variables that are already set.
	_ = godotenv.Load(".env")
	_ = godotenv.Load("../.env")

	if _, err := Environment(); err != nil {
		return err
	}
	if err := ResolveSecrets(ctx, NewAWSSecretsManager); err != nil {
		return err
	}
	return Validate()
}

// Environment returns APP_ENV, which defaults to "development". Unknown
// values are rejected so a typo cannot silently skip production checks.
func Environment() (string, error) {
	env := strings.ToLower(GetEnv("APP_ENV", "development"))
	switch env {
	case "development", "production":
		return env, nil
	default:
		return "", fmt.Errorf("APP_ENV must be \"development\" or \"production\", got %q", env)
	}
}

func IsProduction() bool {
	env, _ := Environment()
	return env == "production"
}

// Validate reports every missing or unsafe setting at once.
func Validate() error {
	var problems []string

	dsn := GetEnv("DATABASE_URL", "")
	if dsn == "" {
		problems = append(problems, "DATABASE_URL is required (see .env.example)")
	}

	_, hasKeyID := os.LookupEnv("AWS_ACCESS_KEY_ID")
	_, hasSecret := os.LookupEnv("AWS_SECRET_ACCESS_KEY")
	if hasKeyID != hasSecret {
		problems = append(problems, "AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY must be set together")
	}

	if IsProduction() {
		if dsn != "" && isDevSecret(dsnPassword(dsn)) {
			problems = append(problems, "DATABASE_URL uses a development password")
		}
		for _, key := range SecretKeys {
			if key != "DATABASE_URL" && isDevSecret(GetEnv(key, "")) {
				problems = append(problems, key+" uses a development value")
			}
		}
		if GetDuration("NOTIFIER_INTERVAL", DefaultNotifierInterval) > 0 && GetEnv("EXPO_ACCESS_TOKEN", "") == "" {
			problems = append(problems, "EXPO_ACCESS_TOKEN is required while the notifier is enabled (or set NOTIFIER_INTERVAL=0)")
		}
	}

	if len(problems) > 0 {
		return errors.New("invalid configuration:\n  - " + strings.Join(problems, "\n  - "))
	}
	return nil
}

func isDevSecret(value string) bool {
	return devSecretValues[value] || strings.HasPrefix(value, "change-me")
}

// dsnPassword extracts the password from a URL or key=value Postgres DSN.
func dsnPassword(dsn string) string {
	if u, err := url.Parse(dsn); err == nil && u.Scheme != "" {
		password, _ := u.User.Password()
		return password
	}
	for _, field := range strings.Fields(dsn) {
		if value, ok := strings.CutPrefix(field, "password="); ok {
			return strings.Trim(value, "'")
		}
	}
	return ""
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

func GetInt(key string, fallback int) int {
	value, exists := os.LookupEnv(key)
	if !exists {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		log.Printf("Invalid integer for %s=%q, using %d", key, value, fallback)
		return fallback
	}
	return parsed
}
