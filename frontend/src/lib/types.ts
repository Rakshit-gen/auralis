export type Genre = { id: string; slug: string; name: string; description: string };
export type Language = { code: string; name: string };

export type Show = {
  id: string;
  creator_id: string;
  creator_name?: string;
  title: string;
  slug: string;
  synopsis: string;
  description: string;
  language_code: string;
  genre_ids: string[];
  genres?: Genre[];
  tags: string[];
  maturity: string;
  cover_image_url: string;
  accent_color: string;
  is_premium: boolean;
  status: string;
  ai_generated: boolean;
  episode_count: number;
  total_duration_sec: number;
  rating_avg: number;
  rating_count: number;
  published_at: string | null;
};

export type Season = {
  id: string;
  show_id: string;
  number: number;
  title: string;
  description: string;
  status: string;
};

export type AudioVariant = { bitrate_kbps: number; codec: string; url?: string; key?: string };

export type Episode = {
  id: string;
  show_id: string;
  season_id: string;
  number: number;
  title: string;
  slug: string;
  synopsis: string;
  script?: string;
  status: string;
  processing: string;
  processing_error?: string;
  is_premium: boolean;
  free_preview_sec: number;
  ai_generated: boolean;
  duration_sec: number;
  published_at: string | null;
};

export type AuthorizeResponse = {
  session_id: string;
  device_id: string | null;
  episode: {
    episode_id: string;
    show_id: string;
    show_slug: string;
    show_title: string;
    episode_title: string;
    episode_number: number;
    duration_sec: number;
    is_premium: boolean;
  };
  hls_master_url: string;
  variants: { bitrate_kbps: number; codec: string; url: string }[];
  resume_position_sec: number;
  preview_only: boolean;
  preview_limit_sec: number;
  expires_at: string;
};

export type ContinueItem = {
  episode_id: string;
  show_id: string;
  position_sec: number;
  duration_sec: number;
  completed: boolean;
  updated_at: string;
};

export type Entitlement = {
  user_id: string;
  plan: string;
  status: string;
  source: string;
  premium_active: boolean;
  granted_at: string;
  expires_at: string | null;
};

export type GenerationJob = {
  id: string;
  kind: string;
  status: string;
  progress: number;
  show_id: string | null;
  episode_id: string | null;
  episode_number: number | null;
  provider: string;
  error: string;
  attempts: number;
  result: Record<string, unknown>;
  created_at: string | null;
  completed_at: string | null;
  events?: { status: string; note: string; at: string }[];
};

export type FeedItem = {
  show_id: string;
  title: string;
  slug: string;
  score: number;
  features: Record<string, number>;
  sources: string[];
  is_premium: boolean;
};
