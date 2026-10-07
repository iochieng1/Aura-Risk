package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeFetcher struct {
	secrets map[string]string
	calls   int
}

func (f *fakeFetcher) GetSecret(_ context.Context, id string) (string, error) {
	f.calls++
	value, ok := f.secrets[id]
	if !ok {
		return "", errors.New("not found")
	}
	return value, nil
}

// clearEnv unsets every variable the config package reads, restoring them
// when the test ends.
func clearEnv(t *testing.T) {
	t.Helper()
	keys := []string{"APP_ENV", "NOTIFIER_INTERVAL"}
	for _, key := range SecretKeys {
		keys = append(keys, key, key+"_FILE")
	}
	for _, key := range keys {
		t.Setenv(key, "")
		os.Unsetenv(key)
	}
}

func TestResolveSecretsFromFile(t *testing.T) {
	clearEnv(t)
	path := filepath.Join(t.TempDir(), "dsn")
	if err := os.WriteFile(path, []byte("postgres://u:p@db/app\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_URL_FILE", path)

	if err := ResolveSecrets(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("DATABASE_URL"); got != "postgres://u:p@db/app" {
		t.Fatalf("DATABASE_URL = %q", got)
	}
}

func TestResolveSecretsRejectsValueAndFile(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("DATABASE_URL_FILE", "/nonexistent")

	if err := ResolveSecrets(context.Background(), nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveSecretsFromSecretManager(t *testing.T) {
	clearEnv(t)
	fetcher := &fakeFetcher{secrets: map[string]string{
		"prod/aurarisk": `{"dsn":"postgres://u:p@db/app","expo":"tok"}`,
		"plain":         "raw-value",
	}}
	t.Setenv("DATABASE_URL", "awssm://prod/aurarisk#dsn")
	t.Setenv("EXPO_ACCESS_TOKEN", "awssm://prod/aurarisk#expo")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "awssm://plain")

	err := ResolveSecrets(context.Background(), func(context.Context) (SecretFetcher, error) { return fetcher, nil })
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"DATABASE_URL":          "postgres://u:p@db/app",
		"EXPO_ACCESS_TOKEN":     "tok",
		"AWS_SECRET_ACCESS_KEY": "raw-value",
	}
	for key, value := range want {
		if got := os.Getenv(key); got != value {
			t.Errorf("%s = %q, want %q", key, got, value)
		}
	}
	if fetcher.calls != 2 {
		t.Errorf("fetched %d times, want 2 (cached per secret id)", fetcher.calls)
	}
}

func TestResolveSecretsSkipsFetcherWithoutReferences(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://u:p@db/app")

	err := ResolveSecrets(context.Background(), func(context.Context) (SecretFetcher, error) {
		t.Fatal("fetcher should not be created")
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestResolveSecretsMissingField(t *testing.T) {
	clearEnv(t)
	fetcher := &fakeFetcher{secrets: map[string]string{"s": `{"a":"b"}`}}
	t.Setenv("DATABASE_URL", "awssm://s#missing")

	err := ResolveSecrets(context.Background(), func(context.Context) (SecretFetcher, error) { return fetcher, nil })
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateRequiresDatabaseURL(t *testing.T) {
	clearEnv(t)
	if err := Validate(); err == nil || !strings.Contains(err.Error(), "DATABASE_URL is required") {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateDevelopmentAllowsLocalCredentials(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://postgres:password@localhost:5432/aurarisk?sslmode=disable")
	t.Setenv("AWS_ACCESS_KEY_ID", "aurarisk")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "aurarisk-dev-secret")

	if err := Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateProductionRejectsDevCredentials(t *testing.T) {
	clearEnv(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://postgres:password@db:5432/aurarisk")
	t.Setenv("AWS_ACCESS_KEY_ID", "aurarisk")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "change-me-objectstore-secret")

	err := Validate()
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"DATABASE_URL uses a development password", "AWS_SECRET_ACCESS_KEY", "EXPO_ACCESS_TOKEN is required"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
}

func TestValidateProductionKeyValueDSN(t *testing.T) {
	clearEnv(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("NOTIFIER_INTERVAL", "0")
	t.Setenv("DATABASE_URL", "host=db user=app password=password dbname=aurarisk")

	if err := Validate(); err == nil {
		t.Fatal("expected error for development password")
	}
}

func TestValidateProductionAcceptsRealConfig(t *testing.T) {
	clearEnv(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://app:s3cr3t-Value@db:5432/aurarisk?sslmode=require")
	t.Setenv("EXPO_ACCESS_TOKEN", "expo-token")

	if err := Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRequiresAWSKeyPair(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://u:p@db/app")
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIA...")

	if err := Validate(); err == nil {
		t.Fatal("expected error")
	}
}

func TestEnvironmentRejectsUnknown(t *testing.T) {
	clearEnv(t)
	t.Setenv("APP_ENV", "prodution")
	if _, err := Environment(); err == nil {
		t.Fatal("expected error")
	}
}

func TestModeratorTokens(t *testing.T) {
	clearEnv(t)
	t.Setenv("MODERATOR_TOKENS", " alice:tok-a , bob:tok-b")
	got, err := ModeratorTokens()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got["alice"] != "tok-a" || got["bob"] != "tok-b" {
		t.Fatalf("ModeratorTokens = %v", got)
	}

	for _, bad := range []string{"alice", "alice:", ":tok", "alice:a,alice:b", "alice:same,bob:same", "auto:tok"} {
		t.Setenv("MODERATOR_TOKENS", bad)
		if _, err := ModeratorTokens(); err == nil {
			t.Errorf("MODERATOR_TOKENS=%q: expected error", bad)
		}
	}
}

func TestValidateProductionRequiresLongModeratorTokens(t *testing.T) {
	clearEnv(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("NOTIFIER_INTERVAL", "0")
	t.Setenv("DATABASE_URL", "postgres://app:s3cr3t-Value@db:5432/aurarisk?sslmode=require")
	t.Setenv("MODERATOR_TOKENS", "alice:short")

	err := Validate()
	if err == nil || !strings.Contains(err.Error(), `token for "alice" must be at least 32 characters`) {
		t.Fatalf("Validate() = %v", err)
	}

	t.Setenv("MODERATOR_TOKENS", "alice:"+strings.Repeat("x", 32))
	if err := Validate(); err != nil {
		t.Fatal(err)
	}
}
