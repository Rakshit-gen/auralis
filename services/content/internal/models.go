package internal

import "time"

// Status values for shows, seasons, and episodes.
const (
	StatusDraft          = "draft"
	StatusReadyForReview = "ready_for_review"
	StatusApproved       = "approved"
	StatusPublished      = "published"
	StatusRejected       = "rejected"
	StatusArchived       = "archived"
)

// Processing states for episode audio.
const (
	ProcNone       = "none"
	ProcQueued     = "queued"
	ProcScript     = "generating_script"
	ProcSynth      = "synthesizing"
	ProcAssembling = "assembling"
	ProcPackaging  = "packaging"
	ProcReady      = "ready"
	ProcFailed     = "failed"
)

// Genre is a catalog category.
type Genre struct {
	ID          string `json:"id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Language is a supported content language.
type Language struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// Creator is a lightweight publisher profile.
type Creator struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	Bio       string    `json:"bio"`
	AvatarURL string    `json:"avatar_url"`
	CreatedAt time.Time `json:"created_at"`
}

// Show is a serialized audio title.
type Show struct {
	ID               string     `json:"id"`
	CreatorID        string     `json:"creator_id"`
	CreatorName      string     `json:"creator_name,omitempty"`
	Title            string     `json:"title"`
	Slug             string     `json:"slug"`
	Synopsis         string     `json:"synopsis"`
	Description      string     `json:"description"`
	LanguageCode     string     `json:"language_code"`
	GenreIDs         []string   `json:"genre_ids"`
	Genres           []Genre    `json:"genres,omitempty"`
	Tags             []string   `json:"tags"`
	Maturity         string     `json:"maturity"`
	CoverImageURL    string     `json:"cover_image_url"`
	AccentColor      string     `json:"accent_color"`
	IsPremium        bool       `json:"is_premium"`
	Status           string     `json:"status"`
	AIGenerated      bool       `json:"ai_generated"`
	EpisodeCount     int        `json:"episode_count"`
	TotalDurationSec int64      `json:"total_duration_sec"`
	RatingAvg        float64    `json:"rating_avg"`
	RatingCount      int64      `json:"rating_count"`
	PublishedAt      *time.Time `json:"published_at"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// Season groups episodes within a show.
type Season struct {
	ID          string     `json:"id"`
	ShowID      string     `json:"show_id"`
	Number      int        `json:"number"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Status      string     `json:"status"`
	PublishedAt *time.Time `json:"published_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

// AudioVariant is one bitrate rendition of an episode's audio.
type AudioVariant struct {
	BitrateKbps int    `json:"bitrate_kbps"`
	Key         string `json:"key"`
	Codec       string `json:"codec"`
	SizeBytes   int64  `json:"size_bytes"`
}

// Episode is a single installment.
type Episode struct {
	ID              string         `json:"id"`
	ShowID          string         `json:"show_id"`
	SeasonID        string         `json:"season_id"`
	Number          int            `json:"number"`
	Title           string         `json:"title"`
	Slug            string         `json:"slug"`
	Synopsis        string         `json:"synopsis"`
	Script          string         `json:"script,omitempty"`
	Status          string         `json:"status"`
	Processing      string         `json:"processing"`
	ProcessingError string         `json:"processing_error,omitempty"`
	IsPremium       bool           `json:"is_premium"`
	FreePreviewSec  int            `json:"free_preview_sec"`
	AIGenerated     bool           `json:"ai_generated"`
	AIJobID         *string        `json:"ai_job_id,omitempty"`
	DurationSec     int            `json:"duration_sec"`
	HLSMasterKey    string         `json:"hls_master_key,omitempty"`
	AudioVariants   []AudioVariant `json:"audio_variants,omitempty"`
	Codec           string         `json:"codec,omitempty"`
	SampleRateHz    int            `json:"sample_rate_hz,omitempty"`
	Channels        int            `json:"channels,omitempty"`
	FileSizeBytes   int64          `json:"file_size_bytes,omitempty"`
	ChecksumSHA256  string         `json:"checksum_sha256,omitempty"`
	PublishedAt     *time.Time     `json:"published_at"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

// ReviewEvent is one entry in an entity's review history.
type ReviewEvent struct {
	ID         int64     `json:"id"`
	EntityType string    `json:"entity_type"`
	EntityID   string    `json:"entity_id"`
	ReviewerID string    `json:"reviewer_id"`
	Action     string    `json:"action"`
	Notes      string    `json:"notes"`
	FromStatus *string   `json:"from_status"`
	ToStatus   string    `json:"to_status"`
	CreatedAt  time.Time `json:"created_at"`
}

func ratingAvg(sum, count int64) float64 {
	if count == 0 {
		return 0
	}
	return float64(sum) / float64(count)
}
