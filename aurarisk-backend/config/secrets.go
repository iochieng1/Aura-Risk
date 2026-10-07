package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// SecretRefPrefix marks an environment value as a reference to an AWS Secrets
// Manager secret: awssm://<secret-id-or-arn>[#<json-field>].
const SecretRefPrefix = "awssm://"

// SecretKeys lists the environment variables that hold credentials. Each one
// can be set directly, read from a mounted file named by <KEY>_FILE, or point
// at AWS Secrets Manager with an awssm:// reference.
var SecretKeys = []string{
	"DATABASE_URL",
	"AWS_ACCESS_KEY_ID",
	"AWS_SECRET_ACCESS_KEY",
	"EXPO_ACCESS_TOKEN",
	"MODERATOR_TOKENS",
}

// SecretFetcher returns the raw value of a secret from a secret manager.
type SecretFetcher interface {
	GetSecret(ctx context.Context, id string) (string, error)
}

// ResolveSecrets replaces file and secret manager references in SecretKeys
// with their values. File references are resolved first so that AWS
// credentials can come from files before the Secrets Manager client is built.
// newFetcher is only called when at least one awssm:// reference is present.
func ResolveSecrets(ctx context.Context, newFetcher func(context.Context) (SecretFetcher, error)) error {
	for _, key := range SecretKeys {
		path, ok := os.LookupEnv(key + "_FILE")
		if !ok || path == "" {
			continue
		}
		if _, set := os.LookupEnv(key); set {
			return fmt.Errorf("%s and %s_FILE are both set; use only one", key, key)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s_FILE: %w", key, err)
		}
		os.Setenv(key, strings.TrimRight(string(data), "\r\n"))
	}

	var fetcher SecretFetcher
	cache := map[string]string{}
	for _, key := range SecretKeys {
		value := os.Getenv(key)
		if !strings.HasPrefix(value, SecretRefPrefix) {
			continue
		}
		id, field, _ := strings.Cut(strings.TrimPrefix(value, SecretRefPrefix), "#")
		if id == "" {
			return fmt.Errorf("%s: empty secret id in %q", key, value)
		}

		raw, ok := cache[id]
		if !ok {
			if fetcher == nil {
				var err error
				if fetcher, err = newFetcher(ctx); err != nil {
					return fmt.Errorf("configuring secret manager: %w", err)
				}
			}
			var err error
			if raw, err = fetcher.GetSecret(ctx, id); err != nil {
				return fmt.Errorf("%s: fetching secret %q: %w", key, id, err)
			}
			cache[id] = raw
		}

		resolved, err := secretField(raw, field)
		if err != nil {
			return fmt.Errorf("%s: secret %q: %w", key, id, err)
		}
		os.Setenv(key, resolved)
	}
	return nil
}

// secretField returns raw unchanged when field is empty, otherwise the named
// string field of raw parsed as a JSON object.
func secretField(raw, field string) (string, error) {
	if field == "" {
		return raw, nil
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return "", fmt.Errorf("field %q requested but secret is not a JSON object", field)
	}
	value, ok := obj[field]
	if !ok {
		return "", fmt.Errorf("field %q not found", field)
	}
	str, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("field %q is not a string", field)
	}
	return str, nil
}
