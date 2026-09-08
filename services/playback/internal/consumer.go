package internal

import (
	"context"

	"github.com/auralis/platform/envelope"
	"github.com/auralis/platform/logging"
)

// ConsumerGroup is the playback service's Kafka consumer group.
const ConsumerGroup = "playback-service"

// Handle keeps the local episode cache in step with the catalog so playback
// authorization does not need a synchronous content lookup on the hot path.
func (a *App) Handle(ctx context.Context, env envelope.Envelope) error {
	switch env.EventType {
	case "content.episode_published":
		var p struct {
			EpisodeID string `json:"episode_id"`
		}
		if err := env.Decode(&p); err != nil {
			return err
		}
		if p.EpisodeID == "" {
			return nil
		}
		// Pull the full episode (including media keys) from content and cache it.
		_, err := a.refreshEpisode(ctx, p.EpisodeID)
		return err

	case "content.episode_unpublished", "content.episode_archived":
		var p struct {
			EpisodeID string `json:"episode_id"`
		}
		if err := env.Decode(&p); err != nil {
			return err
		}
		if p.EpisodeID == "" {
			return nil
		}
		return a.Store.SetEpisodePublished(ctx, p.EpisodeID, false)

	default:
		logging.L(ctx).Debug("ignoring event", "event_type", env.EventType)
		return nil
	}
}

// refreshEpisode forces a content lookup and cache write for an episode.
func (a *App) refreshEpisode(ctx context.Context, episodeID string) (EpisodeMeta, error) {
	var resp struct {
		Episode struct {
			ID             string         `json:"id"`
			ShowID         string         `json:"show_id"`
			Title          string         `json:"title"`
			Number         int            `json:"number"`
			Status         string         `json:"status"`
			IsPremium      bool           `json:"is_premium"`
			FreePreviewSec int            `json:"free_preview_sec"`
			DurationSec    int            `json:"duration_sec"`
			HLSMasterKey   string         `json:"hls_master_key"`
			AudioVariants  []AudioVariant `json:"audio_variants"`
		} `json:"episode"`
		Show struct {
			Slug  string `json:"slug"`
			Title string `json:"title"`
		} `json:"show"`
	}
	if err := a.Content.Get(ctx, "/internal/episodes/"+episodeID, &resp); err != nil {
		return EpisodeMeta{}, err
	}
	m := EpisodeMeta{
		EpisodeID: resp.Episode.ID, ShowID: resp.Episode.ShowID, ShowSlug: resp.Show.Slug,
		ShowTitle: resp.Show.Title, EpisodeTitle: resp.Episode.Title, EpisodeNumber: resp.Episode.Number,
		DurationSec: resp.Episode.DurationSec, IsPremium: resp.Episode.IsPremium,
		FreePreviewSec: resp.Episode.FreePreviewSec, HLSMasterKey: resp.Episode.HLSMasterKey,
		AudioVariants: resp.Episode.AudioVariants, Published: resp.Episode.Status == "published",
	}
	return m, a.Store.UpsertEpisodeCache(ctx, m)
}
