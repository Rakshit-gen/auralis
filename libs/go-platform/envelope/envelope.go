// Package envelope defines the versioned event envelope every Kafka message on
// the platform uses. Payloads are service-specific and validated by the
// consumer; the envelope itself is stable and shared.
package envelope

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Envelope wraps a domain event with routing and tracing metadata.
type Envelope struct {
	EventID       string          `json:"event_id"`
	EventType     string          `json:"event_type"`
	EventVersion  int             `json:"event_version"`
	OccurredAt    time.Time       `json:"occurred_at"`
	Producer      string          `json:"producer"`
	CorrelationID string          `json:"correlation_id"`
	CausationID   string          `json:"causation_id"`
	Payload       json.RawMessage `json:"payload"`
}

// New builds an envelope for a payload that will be JSON-encoded.
func New(eventType string, version int, producer, correlationID, causationID string, payload any) (Envelope, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, err
	}
	if correlationID == "" {
		correlationID = uuid.NewString()
	}
	return Envelope{
		EventID:       uuid.NewString(),
		EventType:     eventType,
		EventVersion:  version,
		OccurredAt:    time.Now().UTC(),
		Producer:      producer,
		CorrelationID: correlationID,
		CausationID:   causationID,
		Payload:       raw,
	}, nil
}

// Validate checks the required fields are populated and well-formed.
func (e Envelope) Validate() error {
	switch {
	case e.EventID == "":
		return errors.New("envelope: event_id is required")
	case e.EventType == "":
		return errors.New("envelope: event_type is required")
	case e.EventVersion < 1:
		return errors.New("envelope: event_version must be >= 1")
	case e.OccurredAt.IsZero():
		return errors.New("envelope: occurred_at is required")
	case e.Producer == "":
		return errors.New("envelope: producer is required")
	case len(e.Payload) == 0:
		return errors.New("envelope: payload is required")
	}
	if _, err := uuid.Parse(e.EventID); err != nil {
		return errors.New("envelope: event_id must be a UUID")
	}
	return nil
}

// Decode unmarshals the payload into v.
func (e Envelope) Decode(v any) error { return json.Unmarshal(e.Payload, v) }

// Bytes returns the JSON encoding of the envelope.
func (e Envelope) Bytes() []byte {
	b, _ := json.Marshal(e)
	return b
}

// Parse decodes and validates an envelope from bytes.
func Parse(b []byte) (Envelope, error) {
	var e Envelope
	if err := json.Unmarshal(b, &e); err != nil {
		return Envelope{}, err
	}
	return e, e.Validate()
}
