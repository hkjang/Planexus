package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Config deliberately contains the complete environment configuration surface.
// Do not add operational settings here; persist them through the admin settings API.
type Config struct {
	PostgresDSN            string
	BootstrapAdmin         string
	BootstrapAdminPassword string
	EncryptionKey          []byte
}

func Load() (Config, error) {
	c := Config{
		PostgresDSN:            strings.TrimSpace(os.Getenv("POSTGRES_DSN")),
		BootstrapAdmin:         strings.TrimSpace(os.Getenv("BOOTSTRAP_ADMIN")),
		BootstrapAdminPassword: os.Getenv("BOOTSTRAP_ADMIN_PASSWORD"),
	}
	if c.PostgresDSN == "" || c.BootstrapAdmin == "" || c.BootstrapAdminPassword == "" {
		return Config{}, errors.New("POSTGRES_DSN, BOOTSTRAP_ADMIN, BOOTSTRAP_ADMIN_PASSWORD and ENCRYPTION_KEY are required")
	}
	if len(c.BootstrapAdminPassword) < 12 {
		return Config{}, errors.New("BOOTSTRAP_ADMIN_PASSWORD must contain at least 12 characters")
	}
	keyText := strings.TrimSpace(os.Getenv("ENCRYPTION_KEY"))
	key, err := base64.StdEncoding.DecodeString(keyText)
	if err != nil || len(key) != 32 {
		return Config{}, fmt.Errorf("ENCRYPTION_KEY must be a base64-encoded 32-byte key")
	}
	c.EncryptionKey = key
	return c, nil
}
