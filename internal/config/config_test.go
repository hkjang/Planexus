package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestLoadAcceptsOnlyRequiredConfiguration(t *testing.T) {
	t.Setenv("POSTGRES_DSN", "postgres://db/planexus")
	t.Setenv("BOOTSTRAP_ADMIN", "admin")
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD", "a-secure-password")
	t.Setenv("ENCRYPTION_KEY", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.EncryptionKey) != 32 || cfg.BootstrapAdmin != "admin" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}

func TestLoadRejectsWeakBootstrapPassword(t *testing.T) {
	t.Setenv("POSTGRES_DSN", "postgres://db/planexus")
	t.Setenv("BOOTSTRAP_ADMIN", "admin")
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD", "short")
	t.Setenv("ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if _, err := Load(); err == nil {
		t.Fatal("expected weak password rejection")
	}
}
