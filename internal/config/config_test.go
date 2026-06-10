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
		Environment:        "production",
		SecretKey:          strings.Repeat("a", MinProductionSecretKeyLen),
		SuperadminPassword: "TestAdmin!99",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidateSettingsDevelopmentAllowsDefaultSecret(t *testing.T) {
	err := validateSettings(&Settings{
		Environment: "development",
		SecretKey:   defaultSecretKey,
		Host:        "127.0.0.1",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidateSettingsNonLoopbackRequiresStrongSecret(t *testing.T) {
	err := validateSettings(&Settings{
		Environment: "development",
		SecretKey:   defaultSecretKey,
		Host:        "0.0.0.0",
	})
	if err == nil {
		t.Fatal("expected error for default secret on non-loopback host")
	}
	err = validateSettings(&Settings{
		Environment:        "development",
		SecretKey:            strings.Repeat("a", MinProductionSecretKeyLen),
		SuperadminPassword:   defaultSuperadminPass,
		Host:                 "0.0.0.0",
	})
	if err == nil {
		t.Fatal("expected error for weak superadmin on non-loopback host")
	}
	err = validateSettings(&Settings{
		Environment:        "development",
		SecretKey:          strings.Repeat("a", MinProductionSecretKeyLen),
		SuperadminPassword: "TestAdmin!99",
		Host:               "0.0.0.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	err = validateSettings(&Settings{
		Environment:        "development",
		SecretKey:          strings.Repeat("a", MinProductionSecretKeyLen),
		SuperadminPassword: "TestAdmin!99",
		Host:               "localhost",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidateSettingsProductionRequiresStrongSuperadminPassword(t *testing.T) {
	base := &Settings{
		Environment: "production",
		SecretKey:   strings.Repeat("a", MinProductionSecretKeyLen),
	}
	err := validateSettings(&Settings{
		Environment:        base.Environment,
		SecretKey:          base.SecretKey,
		SuperadminPassword: defaultSuperadminPass,
	})
	if err == nil {
		t.Fatal("expected error for default superadmin password in production")
	}
	err = validateSettings(&Settings{
		Environment:        base.Environment,
		SecretKey:          base.SecretKey,
		SuperadminPassword: documentedWeakSuperadminPass,
	})
	if err == nil {
		t.Fatal("expected error for documented weak superadmin password in production")
	}
	err = validateSettings(&Settings{
		Environment:        base.Environment,
		SecretKey:          base.SecretKey,
		SuperadminPassword: "short1!",
	})
	if err == nil {
		t.Fatal("expected error for short superadmin password in production")
	}
	err = validateSettings(&Settings{
		Environment:        base.Environment,
		SecretKey:          base.SecretKey,
		SuperadminPassword: "TestAdmin!99",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidateSettingsProductionPostgresTLS(t *testing.T) {
	base := &Settings{
		Environment:        "production",
		SecretKey:          strings.Repeat("a", MinProductionSecretKeyLen),
		SuperadminPassword: "TestAdmin!99",
	}
	err := validateSettings(&Settings{
		Environment:        base.Environment,
		SecretKey:          base.SecretKey,
		SuperadminPassword: base.SuperadminPassword,
		PostgresURL:        "postgresql://u:p@192.168.1.10:5432/bookstorage?sslmode=disable",
	})
	if err != nil {
		t.Fatal("expected LAN private IP with sslmode=disable to be allowed in production:", err)
	}
	err = validateSettings(&Settings{
		Environment:        base.Environment,
		SecretKey:          base.SecretKey,
		SuperadminPassword: base.SuperadminPassword,
		PostgresURL:        "postgresql://u:p@postgres:5432/bookstorage?sslmode=disable",
	})
	if err != nil {
		t.Fatal("expected single-label LAN hostname with sslmode=disable to be allowed:", err)
	}
	err = validateSettings(&Settings{
		Environment:        base.Environment,
		SecretKey:          base.SecretKey,
		SuperadminPassword: base.SuperadminPassword,
		PostgresURL:        "postgresql://u:p@db.example.com:5432/bookstorage?sslmode=disable",
	})
	if err == nil {
		t.Fatal("expected public hostname with sslmode=disable to be rejected in production")
	}
	err = validateSettings(&Settings{
		Environment:        base.Environment,
		SecretKey:          base.SecretKey,
		SuperadminPassword: base.SuperadminPassword,
		PostgresURL:        "postgresql://u:p@8.8.8.8:5432/bookstorage?sslmode=disable",
	})
	if err == nil {
		t.Fatal("expected public IP with sslmode=disable to be rejected in production")
	}
	err = validateSettings(&Settings{
		Environment:        base.Environment,
		SecretKey:          base.SecretKey,
		SuperadminPassword: base.SuperadminPassword,
		PostgresURL:        "postgresql://u:p@192.168.1.10:5432/bookstorage?sslmode=require",
	})
	if err != nil {
		t.Fatal(err)
	}
	err = validateSettings(&Settings{
		Environment:        base.Environment,
		SecretKey:          base.SecretKey,
		SuperadminPassword: base.SuperadminPassword,
		PostgresURL:        "postgresql://u:p@127.0.0.1:5432/bookstorage?sslmode=disable",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidateSettingsGoogleRequiresAllParts(t *testing.T) {
	err := validateSettings(&Settings{
		Environment:        "development",
		SecretKey:          defaultSecretKey,
		GoogleClientID:     "id",
		GoogleClientSecret: "",
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
	err = validateSettings(&Settings{
		Environment:        "development",
		SecretKey:          defaultSecretKey,
		GoogleClientID:     "id",
		GoogleClientSecret: "sec",
	})
	if err == nil {
		t.Fatal("expected error when Google credentials set without public origin")
	}
}

func TestValidateSettingsProductionGoogleRequiresHTTPSOrigin(t *testing.T) {
	err := validateSettings(&Settings{
		Environment:        "production",
		SecretKey:          strings.Repeat("a", MinProductionSecretKeyLen),
		SuperadminPassword: "TestAdmin!99",
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
		SuperadminPassword: "TestAdmin!99",
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

func TestValidateSettingsMailRequiresAllParts(t *testing.T) {
	err := validateSettings(&Settings{
		Environment:          "development",
		SecretKey:            defaultSecretKey,
		MailjetAPIKeyPublic:  "pub",
		MailjetAPIKeyPrivate: "",
		MailFrom:             "noreply@example.com",
	})
	if err == nil {
		t.Fatal("expected error when only some mail variables are set")
	}
	err = validateSettings(&Settings{
		Environment:          "development",
		SecretKey:            defaultSecretKey,
		MailjetAPIKeyPublic:  "pub",
		MailjetAPIKeyPrivate: "priv",
		MailFrom:             "noreply@example.com",
	})
	if err == nil {
		t.Fatal("expected error when mail keys set without public origin")
	}
	err = validateSettings(&Settings{
		Environment:          "development",
		SecretKey:            defaultSecretKey,
		PublicOrigin:         "https://books.example.com",
		MailjetAPIKeyPublic:  "pub",
		MailjetAPIKeyPrivate: "priv",
		MailFrom:             "BookStorage <noreply@books.example.com>",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidateSettingsProductionMailRequiresHTTPSOrigin(t *testing.T) {
	err := validateSettings(&Settings{
		Environment:          "production",
		SecretKey:            strings.Repeat("a", MinProductionSecretKeyLen),
		SuperadminPassword:   "TestAdmin!99",
		PublicOrigin:         "http://books.example.com",
		MailjetAPIKeyPublic:  "pub",
		MailjetAPIKeyPrivate: "priv",
		MailFrom:             "noreply@books.example.com",
	})
	if err == nil {
		t.Fatal("expected error for http public origin with mail in production")
	}
	err = validateSettings(&Settings{
		Environment:          "production",
		SecretKey:            strings.Repeat("a", MinProductionSecretKeyLen),
		SuperadminPassword:   "TestAdmin!99",
		PublicOrigin:         "https://books.example.com",
		MailjetAPIKeyPublic:  "pub",
		MailjetAPIKeyPrivate: "priv",
		MailFrom:             "noreply@books.example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestMailConfigured(t *testing.T) {
	if (&Settings{}).MailConfigured() {
		t.Fatal("expected false for empty settings")
	}
	s := &Settings{
		PublicOrigin:         "https://x.example",
		MailjetAPIKeyPublic:  "pub",
		MailjetAPIKeyPrivate: "priv",
		MailFrom:             "noreply@x.example",
	}
	if !s.MailConfigured() {
		t.Fatal("expected mail configured")
	}
}
