package internal

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/auralis/platform/errcodes"
	"github.com/auralis/platform/httpx"
	"github.com/google/uuid"
)

type uploadURLReq struct {
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
}

// allowedUploadTypes are the source audio formats a creator may upload. On
// confirm the stored object has to match the declared type and size limit.
// The ai-media worker then probes it with ffprobe before packaging.
var allowedUploadTypes = map[string]string{
	"audio/mpeg":  "mp3",
	"audio/mp4":   "m4a",
	"audio/x-m4a": "m4a",
	"audio/wav":   "wav",
	"audio/x-wav": "wav",
	"audio/flac":  "flac",
	"audio/ogg":   "ogg",
}

func (a *App) createUploadURL(w http.ResponseWriter, r *http.Request) {
	e, err := a.episodeOwned(w, r)
	if err != nil {
		return
	}
	var req uploadURLReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	ext, ok := allowedUploadTypes[strings.ToLower(strings.TrimSpace(req.ContentType))]
	if !ok {
		httpx.Error(w, r, errcodes.BadRequest("content_type must be one of mp3, m4a, wav, flac, ogg"))
		return
	}
	if req.SizeBytes <= 0 || req.SizeBytes > a.MaxUploadMB*1024*1024 {
		httpx.Error(w, r, errcodes.BadRequest(
			fmt.Sprintf("size_bytes must be between 1 and %d MB", a.MaxUploadMB)))
		return
	}

	key := fmt.Sprintf("source/%s/%s/%s.%s", e.ShowID, e.ID, uuid.NewString(), ext)
	url, err := a.Objects.PresignPut(r.Context(), key, a.UploadTTL, req.ContentType, req.SizeBytes)
	if err != nil {
		httpx.Error(w, r, errcodes.New(http.StatusBadGateway, errcodes.Unavailable, "object storage unavailable"))
		return
	}
	uploadID, err := a.Store.CreateUpload(r.Context(), e.ID, key, req.ContentType)
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not record upload"))
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"upload_id": uploadID, "url": url, "method": "PUT", "object_key": key,
		"headers": map[string]string{
			"Content-Type": req.ContentType, "Content-Length": fmt.Sprint(req.SizeBytes),
		},
		"expires_at": time.Now().Add(a.UploadTTL).UTC(),
	})
}

type confirmUploadReq struct {
	UploadID string `json:"upload_id"`
}

// confirmUpload validates the uploaded object exists with a plausible size, then
// emits content.audio_uploaded so the ai-media service packages it into HLS.
func (a *App) confirmUpload(w http.ResponseWriter, r *http.Request) {
	e, err := a.episodeOwned(w, r)
	if err != nil {
		return
	}
	var req confirmUploadReq
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	episodeID, key, declaredType, err := a.Store.PendingUpload(r.Context(), req.UploadID)
	if err != nil || episodeID != e.ID {
		httpx.Error(w, r, errcodes.Missing("upload not found or already confirmed"))
		return
	}

	size, storedType, err := a.Objects.Stat(r.Context(), key)
	if err != nil {
		httpx.Error(w, r, errcodes.BadRequest("uploaded object not found in storage"))
		return
	}
	if msg := a.checkUpload(size, storedType, declaredType); msg != "" {
		// Don't keep what was rejected, and don't let it be confirmed later.
		_ = a.Objects.Remove(r.Context(), key)
		_ = a.Store.RejectUpload(r.Context(), req.UploadID)
		httpx.Error(w, r, errcodes.BadRequest(msg))
		return
	}

	ctx := r.Context()
	tx, err := a.Store.Pool().Begin(ctx)
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("database unavailable"))
		return
	}
	defer tx.Rollback(ctx)

	if err := a.Store.ConfirmUpload(ctx, tx, req.UploadID, size); err != nil {
		httpx.Error(w, r, errcodes.Missing("upload not found or already confirmed"))
		return
	}
	if err := a.Store.SetEpisodeProcessing(ctx, tx, e.ID, ProcQueued, ""); err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not queue processing"))
		return
	}
	err = a.enqueue(ctx, tx, "content.audio_uploaded", 1, e.ShowID, map[string]any{
		"episode_id": e.ID, "show_id": e.ShowID, "source_key": key, "size_bytes": size,
		"source": "upload",
	})
	if err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not enqueue processing event"))
		return
	}
	if err := tx.Commit(ctx); err != nil {
		httpx.Error(w, r, errcodes.Unexpected("could not confirm upload"))
		return
	}
	metric("audio_uploaded", "success")
	httpx.JSON(w, http.StatusAccepted, map[string]any{"episode_id": e.ID, "processing": ProcQueued})
}

// checkUpload returns why a stored upload is unacceptable, or "".
func (a *App) checkUpload(size int64, storedType, declaredType string) string {
	switch {
	case size < 1024:
		return "uploaded object is too small to be audio"
	case size > a.MaxUploadMB*1024*1024:
		return fmt.Sprintf("uploaded object is larger than %d MB", a.MaxUploadMB)
	case baseType(storedType) != baseType(declaredType):
		return "uploaded object's content type does not match the one requested"
	}
	return ""
}

func baseType(ct string) string {
	t, _, _ := strings.Cut(ct, ";")
	return strings.ToLower(strings.TrimSpace(t))
}
