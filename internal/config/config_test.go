package config

import (
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
