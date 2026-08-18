// Package config loads runtime configuration from the environment.
package config

import (
	"fmt"
	"os"
)

// Buckets are the object-storage namespaces used by the application.
// Slice 0 only needs them to exist; later slices write to them.
var Buckets = []string{"meal-images", "avatars", "chat-media", "user-stickers"}

type Config struct {
	HTTPAddr    string
	DatabaseURL string
	// StorageEmulatorHost points at fake-gcs-server, e.g. "http://fake-gcs:4443".
	// V0.1 never talks to real Google Cloud Storage.
	StorageEmulatorHost string
	// WebOrigin is the single browser origin allowed to call the API with
	// credentials. Sessions are cookie-based, so this cannot be a wildcard.
	WebOrigin string
	// SecureCookies must be true wherever the site is served over HTTPS.
	SecureCookies bool
}

func Load() (Config, error) {
	c := Config{
		HTTPAddr:            envOr("CHIBAN_HTTP_ADDR", ":8080"),
		DatabaseURL:         os.Getenv("CHIBAN_DATABASE_URL"),
		StorageEmulatorHost: os.Getenv("CHIBAN_STORAGE_EMULATOR_HOST"),
		WebOrigin:           envOr("CHIBAN_WEB_ORIGIN", "http://localhost:3000"),
		SecureCookies:       os.Getenv("CHIBAN_SECURE_COOKIES") == "true",
	}
	if c.DatabaseURL == "" {
		return Config{}, fmt.Errorf("CHIBAN_DATABASE_URL is required")
	}
	if c.StorageEmulatorHost == "" {
		return Config{}, fmt.Errorf("CHIBAN_STORAGE_EMULATOR_HOST is required")
	}
	return c, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
