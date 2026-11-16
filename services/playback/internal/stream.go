package internal

import (
	"bufio"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/auralis/platform/errcodes"
	"github.com/auralis/platform/httpx"
	"github.com/go-chi/chi/v5"
)

// Playlists are served from here rather than straight from storage. The
// bucket is public, and a playlist lists every segment of an episode, so
// handing a listener its storage URL would hand over the whole episode. Here
// a preview listener gets a playlist that stops at the preview limit, and
// nobody learns where the stored playlists are.
//
// hls.js and Safari fetch playlists without an Authorization header, so the
// URL carries a short-lived signed token instead.

var playlistName = regexp.MustCompile(`^[A-Za-z0-9_-]+\.m3u8$`)

// streamToken signs which episode a stream URL is for, the preview limit (0
// for the whole episode) and when it stops working.
func (a *App) streamToken(episodeID string, previewSec int, expires time.Time) string {
	body := fmt.Sprintf("%s.%d.%d", episodeID, previewSec, expires.Unix())
	return base64.RawURLEncoding.EncodeToString([]byte(body)) + "." + a.signStream(body)
}

func (a *App) signStream(body string) string {
	mac := hmac.New(sha256.New, []byte(a.StreamSecret))
	mac.Write([]byte("stream\n" + body))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (a *App) parseStreamToken(token string) (episodeID string, previewSec int, err error) {
	invalid := errcodes.Unauthed("stream link is invalid or has expired")
	enc, sig, ok := strings.Cut(token, ".")
	raw, derr := base64.RawURLEncoding.DecodeString(enc)
	if !ok || derr != nil || !hmac.Equal([]byte(sig), []byte(a.signStream(string(raw)))) {
		return "", 0, invalid
	}
	parts := strings.Split(string(raw), ".")
	if len(parts) != 3 {
		return "", 0, invalid
	}
	previewSec, perr := strconv.Atoi(parts[1])
	exp, eerr := strconv.ParseInt(parts[2], 10, 64)
	if perr != nil || eerr != nil || time.Now().Unix() > exp {
		return "", 0, invalid
	}
	return parts[0], previewSec, nil
}

// streamPath is what authorize hands the client, relative to the API base.
func (a *App) streamPath(episodeID string, previewSec int) string {
	return "/playback/stream/" + a.streamToken(episodeID, previewSec, time.Now().Add(a.SignedTTL)) + "/" +
		"master.m3u8"
}

func (a *App) stream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	episodeID, previewSec, err := a.parseStreamToken(chi.URLParam(r, "token"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	file := chi.URLParam(r, "file")
	meta, err := a.episodeMeta(ctx, episodeID)
	if err != nil || !meta.Published || meta.HLSMasterKey == "" || !playlistName.MatchString(file) {
		httpx.Error(w, r, errcodes.Missing("playlist not found"))
		return
	}
	dir := path.Dir(meta.HLSMasterKey)
	isMaster := file == path.Base(meta.HLSMasterKey)
	if !isMaster && !isVariant(meta, dir+"/"+file) {
		httpx.Error(w, r, errcodes.Missing("playlist not found"))
		return
	}
	body, err := a.Objects.GetBytes(ctx, dir+"/"+file)
	if err != nil {
		httpx.Error(w, r, errcodes.New(http.StatusBadGateway, errcodes.Unavailable, "media storage unavailable"))
		return
	}
	// The master only names the variant playlists, which resolve back to this
	// route. A variant names segments, which go straight to storage.
	if !isMaster {
		if body, err = a.rewriteMediaPlaylist(ctx, body, dir, previewSec); err != nil {
			httpx.Error(w, r, errcodes.New(http.StatusBadGateway, errcodes.Unavailable, "media storage unavailable"))
			return
		}
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "private, no-store")
	_, _ = w.Write(body)
}

func isVariant(meta EpisodeMeta, key string) bool {
	for _, v := range meta.AudioVariants {
		if v.Key == key {
			return true
		}
	}
	return false
}

// rewriteMediaPlaylist points each segment at storage and, for a preview,
// drops every segment that starts at or after the preview limit.
func (a *App) rewriteMediaPlaylist(ctx context.Context, body []byte, dir string, previewSec int) ([]byte, error) {
	var out bytes.Buffer
	elapsed, segDur := 0.0, 0.0
	sc := bufio.NewScanner(bytes.NewReader(body))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "#EXT-X-ENDLIST"):
			continue // added once at the end
		case strings.HasPrefix(line, "#EXTINF:"):
			d, _, _ := strings.Cut(strings.TrimPrefix(line, "#EXTINF:"), ",")
			segDur, _ = strconv.ParseFloat(d, 64)
			if previewSec > 0 && elapsed >= float64(previewSec) {
				out.WriteString("#EXT-X-ENDLIST\n")
				return out.Bytes(), nil
			}
		case !strings.HasPrefix(line, "#"):
			if strings.Contains(line, "/") || strings.Contains(line, "..") {
				return nil, fmt.Errorf("unexpected segment uri %q", line)
			}
			u, err := a.mediaURL(ctx, dir+"/"+line)
			if err != nil {
				return nil, err
			}
			line = u
			elapsed += segDur
		}
		out.WriteString(line + "\n")
	}
	out.WriteString("#EXT-X-ENDLIST\n")
	return out.Bytes(), sc.Err()
}
