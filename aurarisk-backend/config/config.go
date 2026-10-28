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

	"aurarisk-backend/internal/logging"
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

	responders, err := ResponderTokens()
	if err != nil {
		problems = append(problems, err.Error())
	}

	webhooks, err := CountyWebhooks()
	if err != nil {
		problems = append(problems, err.Error())
	}

	if GetEnv("CHANNEL_CALLBACK_SECRET", "") != "" && GetEnv("CHANNEL_PHONE_SALT", "") == "" {
		problems = append(problems, "CHANNEL_PHONE_SALT is required while CHANNEL_CALLBACK_SECRET is set")
	}
	if (GetEnv("AT_API_KEY", "") == "") != (GetEnv("AT_USERNAME", "") == "") {
		problems = append(problems, "AT_USERNAME and AT_API_KEY must be set together")
	}

	if err := logging.CheckFormat(LogFormat()); err != nil {
		problems = append(problems, err.Error())
	}

	if IsProduction() {
		for name, token := range moderators {
			if len(token) < minModeratorTokenLength {
				problems = append(problems, fmt.Sprintf("MODERATOR_TOKENS: token for %q must be at least %d characters", name, minModeratorTokenLength))
			}
		}
		for name, r := range responders {
			if len(r.Token) < minModeratorTokenLength {
				problems = append(problems, fmt.Sprintf("RESPONDER_TOKENS: token for %q must be at least %d characters", name, minModeratorTokenLength))
			}
		}
		if len(webhooks) > 0 && len(GetEnv("COUNTY_WEBHOOK_SECRET", "")) < minModeratorTokenLength {
			problems = append(problems, fmt.Sprintf("COUNTY_WEBHOOK_SECRET must be at least %d characters while COUNTY_WEBHOOKS is set", minModeratorTokenLength))
		}
		if secret := GetEnv("CHANNEL_CALLBACK_SECRET", ""); secret != "" {
			if len(secret) < minModeratorTokenLength {
				problems = append(problems, fmt.Sprintf("CHANNEL_CALLBACK_SECRET must be at least %d characters", minModeratorTokenLength))
			}
			if len(GetEnv("CHANNEL_PHONE_SALT", "")) < minModeratorTokenLength {
				problems = append(problems, fmt.Sprintf("CHANNEL_PHONE_SALT must be at least %d characters while SMS/USSD is enabled", minModeratorTokenLength))
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

// LogFormat returns LOG_FORMAT: "json" by default in production, so logs can
// be shipped and searched, and "text" in development.
func LogFormat() string {
	fallback := logging.FormatText
	if IsProduction() {
		fallback = logging.FormatJSON
	}
	return strings.ToLower(GetEnv("LOG_FORMAT", fallback))
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

// Responder is a county responder from RESPONDER_TOKENS.
type Responder struct {
	// County as written in the setting; the handlers check it is one of
	// Kenya's 47 and store the official name.
	County string
	Token  string
}

// ResponderTokens parses RESPONDER_TOKENS ("name:County:token,...") into a
// map of responder name to county and token. Names are recorded in the
// response audit trail. An empty value disables the responder API.
func ResponderTokens() (map[string]Responder, error) {
	raw := strings.TrimSpace(GetEnv("RESPONDER_TOKENS", ""))
	responders := map[string]Responder{}
	if raw == "" {
		return responders, nil
	}
	seen := map[string]bool{}
	for _, entry := range strings.Split(raw, ",") {
		parts := strings.SplitN(strings.TrimSpace(entry), ":", 3)
		if len(parts) != 3 {
			return nil, errors.New(`RESPONDER_TOKENS must be a comma-separated list of "name:County:token"`)
		}
		name, county, token := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), strings.TrimSpace(parts[2])
		if name == "" || county == "" || token == "" {
			return nil, errors.New(`RESPONDER_TOKENS must be a comma-separated list of "name:County:token"`)
		}
		if _, dup := responders[name]; dup {
			return nil, fmt.Errorf("RESPONDER_TOKENS: responder %q is listed twice", name)
		}
		if seen[token] {
			return nil, errors.New("RESPONDER_TOKENS: two responders share a token")
		}
		seen[token] = true
		responders[name] = Responder{County: county, Token: token}
	}
	return responders, nil
}

// CountyWebhooks parses COUNTY_WEBHOOKS ("County=https://...,...") into a map
// of county, as written, to the URL notified when a report there is verified.
func CountyWebhooks() (map[string]string, error) {
	hooks := map[string]string{}
	for _, entry := range GetList("COUNTY_WEBHOOKS", nil) {
		county, rawURL, ok := strings.Cut(entry, "=")
		county, rawURL = strings.TrimSpace(county), strings.TrimSpace(rawURL)
		if !ok || county == "" || rawURL == "" {
			return nil, errors.New(`COUNTY_WEBHOOKS must be a comma-separated list of "County=URL"`)
		}
		u, err := url.Parse(rawURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return nil, fmt.Errorf("COUNTY_WEBHOOKS: %q is not an http(s) URL", rawURL)
		}
		if IsProduction() && u.Scheme != "https" {
			return nil, fmt.Errorf("COUNTY_WEBHOOKS: %q must use https in production", rawURL)
		}
		if _, dup := hooks[county]; dup {
			return nil, fmt.Errorf("COUNTY_WEBHOOKS: county %q is listed twice", county)
		}
		hooks[county] = rawURL
	}
	return hooks, nil
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
