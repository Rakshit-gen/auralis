// Package config reads deployment configuration from environment variables.
// There are no defaults for addresses or secrets; a missing required value is a
// startup error so misconfiguration fails fast instead of at first request.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Missing is returned by Load when required keys are absent.
type Missing struct{ Keys []string }

func (m *Missing) Error() string {
	return "missing required environment variables: " + strings.Join(m.Keys, ", ")
}

// Reader collects lookups and reports every missing required key at once.
type Reader struct {
	missing []string
}

func New() *Reader { return &Reader{} }

// Require returns the value for key or records it as missing.
func (r *Reader) Require(key string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		r.missing = append(r.missing, key)
	}
	return v
}

// Optional returns the value for key or fallback when unset.
func (r *Reader) Optional(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

// OptionalInt parses an integer env value or returns fallback.
func (r *Reader) OptionalInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		r.missing = append(r.missing, fmt.Sprintf("%s (not an integer: %q)", key, v))
		return fallback
	}
	return n
}

// OptionalDuration parses a Go duration string or returns fallback.
func (r *Reader) OptionalDuration(key string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		r.missing = append(r.missing, fmt.Sprintf("%s (not a duration: %q)", key, v))
		return fallback
	}
	return d
}

// OptionalBool parses 1/true/yes/on (case-insensitive) as true.
func (r *Reader) OptionalBool(key string, fallback bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if v == "" {
		return fallback
	}
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		// Sibling parsers record a bad value; a silent fallback here would let a
		// typo like RUN_CONSUMER=treu quietly disable a consumer.
		r.missing = append(r.missing, fmt.Sprintf("%s (not a boolean: %q)", key, v))
		return fallback
	}
}

// CSV splits a comma separated env value, trimming spaces and dropping empties.
func (r *Reader) CSV(key string, fallback []string) []string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Err returns a *Missing error if any required key was absent, else nil.
func (r *Reader) Err() error {
	if len(r.missing) == 0 {
		return nil
	}
	return &Missing{Keys: r.missing}
}
