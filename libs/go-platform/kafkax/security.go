// Kafka transport security. Local and CI brokers speak PLAINTEXT and need none
// of this; managed brokers (Redpanda Cloud, MSK, Confluent) require SASL over
// TLS. The settings come from the environment so the wiring is identical across
// every producer and consumer:
//
//	KAFKA_SASL_MECHANISM   PLAIN | SCRAM-SHA-256 | SCRAM-SHA-512   (empty => PLAINTEXT)
//	KAFKA_SASL_USERNAME    broker principal
//	KAFKA_SASL_PASSWORD    broker secret
//	KAFKA_TLS_ENABLED      1/true to force TLS; implied whenever a mechanism is set
//	KAFKA_TLS_SKIP_VERIFY  1/true to skip certificate verification (test only)
//
// When KAFKA_SASL_MECHANISM is empty every function here returns the zero value
// and the clients behave exactly as before.
package kafkax

import (
	"crypto/tls"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl"
	"github.com/segmentio/kafka-go/sasl/plain"
	"github.com/segmentio/kafka-go/sasl/scram"
)

func envBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// saslMechanism builds the configured SASL mechanism, or nil for PLAINTEXT.
// A malformed configuration panics at startup rather than failing at first
// publish, matching the fail-fast contract of the config package.
func saslMechanism() sasl.Mechanism {
	mech := strings.ToUpper(strings.TrimSpace(os.Getenv("KAFKA_SASL_MECHANISM")))
	if mech == "" {
		return nil
	}
	user := os.Getenv("KAFKA_SASL_USERNAME")
	pass := os.Getenv("KAFKA_SASL_PASSWORD")
	if user == "" || pass == "" {
		panic("kafkax: KAFKA_SASL_MECHANISM is set but KAFKA_SASL_USERNAME/PASSWORD are empty")
	}
	switch mech {
	case "PLAIN":
		return plain.Mechanism{Username: user, Password: pass}
	case "SCRAM-SHA-256":
		m, err := scram.Mechanism(scram.SHA256, user, pass)
		if err != nil {
			panic(fmt.Sprintf("kafkax: SCRAM-SHA-256 setup: %v", err))
		}
		return m
	case "SCRAM-SHA-512":
		m, err := scram.Mechanism(scram.SHA512, user, pass)
		if err != nil {
			panic(fmt.Sprintf("kafkax: SCRAM-SHA-512 setup: %v", err))
		}
		return m
	default:
		panic(fmt.Sprintf("kafkax: unsupported KAFKA_SASL_MECHANISM %q (want PLAIN, SCRAM-SHA-256 or SCRAM-SHA-512)", mech))
	}
}

// tlsConfig returns the TLS config for the broker connection, or nil for
// plaintext. TLS is on when KAFKA_TLS_ENABLED is truthy or whenever a SASL
// mechanism is configured (cloud brokers reject SASL over a cleartext socket).
func tlsConfig() *tls.Config {
	if !envBool("KAFKA_TLS_ENABLED") && strings.TrimSpace(os.Getenv("KAFKA_SASL_MECHANISM")) == "" {
		return nil
	}
	return &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: envBool("KAFKA_TLS_SKIP_VERIFY"), //nolint:gosec // opt-in, test only
	}
}

// transport returns a *kafka.Transport carrying the configured SASL and TLS
// settings, for use by writers. Safe to share across writers.
func transport() *kafka.Transport {
	return &kafka.Transport{
		SASL:        saslMechanism(),
		TLS:         tlsConfig(),
		DialTimeout: 10 * time.Second,
		IdleTimeout: 30 * time.Second,
	}
}

// dialer returns a *kafka.Dialer carrying the configured SASL and TLS settings,
// for use by readers and by administrative dial calls.
func dialer() *kafka.Dialer {
	return &kafka.Dialer{
		Timeout:       10 * time.Second,
		DualStack:     true,
		SASLMechanism: saslMechanism(),
		TLS:           tlsConfig(),
	}
}
