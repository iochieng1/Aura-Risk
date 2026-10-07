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

	for _, origin := range CORSOrigins() {
		if err := checkOrigin(origin); err != nil {
			problems = append(problems, "CORS_ALLOWED_ORIGINS: "+err.Error())
		}
	}

	moderators, err := ModeratorTokens()
	if err != nil {
		problems = append(problems, err.Error())
	}

	if IsProduction() {
		for name, token := range moderators {
			if len(token) < minModeratorTokenLength {
				problems = append(problems, fmt.Sprintf("MODERATOR_TOKENS: token for %q must be at least %d characters", name, minModeratorTokenLength))
			}
		}
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

const minModeratorTokenLength = 32

// ModeratorTokens parses MODERATOR_TOKENS ("name:token,name:token") into a
// map of moderator name to token. Names are recorded in the moderation audit
// trail. An empty value disables the moderation API.
func ModeratorTokens() (map[string]string, error) {
	raw := strings.TrimSpace(GetEnv("MODERATOR_TOKENS", ""))
	tokens := map[string]string{}
	if raw == "" {
		return tokens, nil
	}
	seen := map[string]bool{}
	for _, entry := range strings.Split(raw, ",") {
		name, token, ok := strings.Cut(strings.TrimSpace(entry), ":")
		name, token = strings.TrimSpace(name), strings.TrimSpace(token)
		if !ok || name == "" || token == "" {
			return nil, errors.New(`MODERATOR_TOKENS must be a comma-separated list of "name:token"`)
		}
		if name == "auto" {
			return nil, errors.New(`MODERATOR_TOKENS: "auto" is reserved for automatic verification`)
		}
		if _, dup := tokens[name]; dup {
			return nil, fmt.Errorf("MODERATOR_TOKENS: moderator %q is listed twice", name)
		}
		if seen[token] {
			return nil, errors.New("MODERATOR_TOKENS: two moderators share a token")
		}
		seen[token] = true
		tokens[name] = token
	}
	return tokens, nil
}

// GetList reads a comma-separated list, ignoring blanks. Unset uses fallback;
// set but empty gives an empty list.
func GetList(key string, fallback []string) []string {
	value, exists := os.LookupEnv(key)
	if !exists {
		return fallback
	}
	var out []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// DefaultCORSOrigin is the Vite dev server.
const DefaultCORSOrigin = "http://localhost:5173"

// CORSOrigins returns the browser origins allowed to call the API, from
// CORS_ALLOWED_ORIGINS. Unset allows the local Vite dev server only.
func CORSOrigins() []string {
	return GetList("CORS_ALLOWED_ORIGINS", []string{DefaultCORSOrigin})
}

// checkOrigin accepts a bare scheme://host[:port]. A wildcard is refused
// because the API allows credentials.
func checkOrigin(origin string) error {
	if origin == "*" {
		return errors.New(`"*" is not allowed; list each web app origin`)
	}
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return fmt.Errorf("%q must look like https://app.example.org (scheme and host only)", origin)
	}
	if strings.HasSuffix(origin, "/") {
		return fmt.Errorf("%q must not end with a slash", origin)
	}
	return nil
}
