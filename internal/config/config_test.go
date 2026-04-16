package config

import (
	"net/url"
	"strings"
	"testing"
)

func TestValidateSettingsProductionRequiresStrongSecret(t *testing.T) {
	err := validateSettings(&Settings{
		Environment: "production",
		SecretKey:   defaultSecretKey,
	})
	if err == nil {
		t.Fatal("expected error for default secret in production")
	}
	err = validateSettings(&Settings{
		Environment: "production",
		SecretKey:   "short",
	})
	if err == nil {
		t.Fatal("expected error for short secret in production")
	}
	err = validateSettings(&Settings{
		Environment: "production",
		SecretKey:   strings.Repeat("a", MinProductionSecretKeyLen),
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidateSettingsDevelopmentAllowsDefaultSecret(t *testing.T) {
	err := validateSettings(&Settings{
		Environment: "development",
		SecretKey:   defaultSecretKey,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidateSettingsGoogleRequiresAllParts(t *testing.T) {
	err := validateSettings(&Settings{
		Environment:        "development",
		SecretKey:          defaultSecretKey,
		PublicOrigin:       "https://a.example",
		GoogleClientID:     "",
		GoogleClientSecret: "x",
	})
	if err == nil {
		t.Fatal("expected error when only some Google OAuth variables are set")
	}
	err = validateSettings(&Settings{
		Environment:        "development",
		SecretKey:          defaultSecretKey,
		PublicOrigin:       "https://a.example",
		GoogleClientID:     "id",
		GoogleClientSecret: "sec",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidateSettingsProductionGoogleRequiresHTTPSOrigin(t *testing.T) {
	err := validateSettings(&Settings{
		Environment:        "production",
		SecretKey:          strings.Repeat("a", MinProductionSecretKeyLen),
		PublicOrigin:       "http://books.example.com",
		GoogleClientID:     "id",
		GoogleClientSecret: "sec",
	})
	if err == nil {
		t.Fatal("expected error for http public origin with Google in production")
	}
	err = validateSettings(&Settings{
		Environment:        "production",
		SecretKey:          strings.Repeat("a", MinProductionSecretKeyLen),
		PublicOrigin:       "https://books.example.com",
		GoogleClientID:     "id",
		GoogleClientSecret: "sec",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestNormalizePostgresURLForLibPQPreferToDisable(t *testing.T) {
	out, err := NormalizePostgresURLForLibPQ("postgresql://u:p@127.0.0.1:5432/dbname?sslmode=prefer")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	if u.Query().Get("sslmode") != "disable" {
		t.Fatalf("got query %q", u.RawQuery)
	}
}

func TestNormalizePostgresURLForLibPQUnsupportedMode(t *testing.T) {
	_, err := NormalizePostgresURLForLibPQ("postgresql://u:p@127.0.0.1:5432/db?sslmode=notamode")
	if err == nil {
		t.Fatal("expected error")
	}
}
