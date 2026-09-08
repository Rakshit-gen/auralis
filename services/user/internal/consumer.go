package internal

import (
	"context"
	"time"

	"github.com/auralis/platform/envelope"
	"github.com/auralis/platform/logging"
)

// Consumer group id for the user service.
const ConsumerGroup = "user-service"

// Handle processes one domain event. It is idempotent: registration is an
// upsert and listening history uses GREATEST merges, so a redelivered event is
// harmless even if the processed-events check is bypassed.
func (a *App) Handle(ctx context.Context, env envelope.Envelope) error {
	switch env.EventType {
	case "user.registered":
		var p struct {
			UserID      string `json:"user_id"`
			Email       string `json:"email"`
			DisplayName string `json:"display_name"`
		}
		if err := env.Decode(&p); err != nil {
			return err
		}
		return a.Store.EnsureProfile(ctx, p.UserID, p.DisplayName, p.Email)

	case "playback.progress", "playback.completed":
		var p struct {
			UserID      string    `json:"user_id"`
			EpisodeID   string    `json:"episode_id"`
			ShowID      string    `json:"show_id"`
			PositionSec int       `json:"position_sec"`
			Completed   bool      `json:"completed"`
			OccurredAt  time.Time `json:"occurred_at"`
		}
		if err := env.Decode(&p); err != nil {
			return err
		}
		if p.UserID == "" || p.EpisodeID == "" {
			return nil
		}
		at := p.OccurredAt
		if at.IsZero() {
			at = env.OccurredAt
		}
		completed := p.Completed || env.EventType == "playback.completed"
		return a.Store.RecordListening(ctx, p.UserID, p.EpisodeID, p.ShowID, p.PositionSec, completed, at)

	default:
		logging.L(ctx).Debug("ignoring event", "event_type", env.EventType)
		return nil
	}
}
