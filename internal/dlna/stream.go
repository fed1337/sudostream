package dlna

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"sudoStream/internal/auth"
	"sudoStream/internal/mediafs"
	"sudoStream/internal/observability"
	"sudoStream/internal/playback"
	"time"
)

// accessChecker is the ACL surface used by signed stream GETs.
type accessChecker interface {
	CanRead(ctx context.Context, user auth.PublicUser, rawPath string) (bool, error)
}

// userLoader loads the TV principal for signed stream URLs.
type userLoader interface {
	GetUserByID(ctx context.Context, userID string) (*auth.User, error)
}

// StreamHandler serves signed progressive play URLs on the DLNA port.
type StreamHandler struct {
	Access  accessChecker
	Media   *mediafs.Service
	Auth    userLoader
	SignKey []byte
	Now     func() time.Time
}

func (h *StreamHandler) ServeHTTP( //nolint:cyclop // auth + mode switch
	writer http.ResponseWriter,
	request *http.Request,
) {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)

		return
	}

	mediaPath := strings.TrimPrefix(request.URL.Path, "/dlna/stream/")
	mediaPath = pathUnescapeKeepSlash(mediaPath)
	mediaPath = strings.TrimPrefix(mediaPath, "/")

	query := request.URL.Query()
	now := time.Now().UTC()
	if h.Now != nil {
		now = h.Now()
	}

	mode, err := VerifyStream(
		h.SignKey,
		query.Get("uid"),
		mediaPath,
		query.Get("m"),
		query.Get("exp"),
		query.Get("sig"),
		now,
	)
	if err != nil {
		observability.RecordDLNAStreamRequest("forbidden")
		http.Error(writer, "forbidden", http.StatusForbidden)

		return
	}

	user, loadErr := h.Auth.GetUserByID(request.Context(), query.Get("uid"))
	if loadErr != nil || user == nil || !user.Enabled || user.Role != auth.RoleTV {
		observability.RecordDLNAStreamRequest("forbidden")
		http.Error(writer, "forbidden", http.StatusForbidden)

		return
	}

	public := auth.PublicUser{ID: user.ID, Email: user.Email, Role: user.Role}
	allowed, aclErr := h.Access.CanRead(request.Context(), public, mediaPath)
	if aclErr != nil || !allowed {
		observability.RecordDLNAStreamRequest("forbidden")
		http.Error(writer, "forbidden", http.StatusForbidden)

		return
	}

	switch mode {
	case StreamDirect:
		observability.RecordDLNAStreamRequest("success")
		h.serveDirect(writer, request, mediaPath)
	case StreamRemux, StreamTranscode:
		observability.RecordDLNAStreamRequest("success")
		h.serveMPEGTS(writer, request, mediaPath, mode == StreamTranscode)
	default:
		observability.RecordDLNAStreamRequest("forbidden")
		http.Error(writer, "forbidden", http.StatusForbidden)
	}
}

func (h *StreamHandler) serveDirect(
	writer http.ResponseWriter,
	request *http.Request,
	mediaPath string,
) {
	file, info, err := h.Media.OpenFile(mediaPath)
	if err != nil {
		http.Error(writer, "not found", http.StatusNotFound)

		return
	}
	defer func() { _ = file.Close() }()

	method := string(playback.MethodDirectPlay)
	observability.IncPlaybackStream(method, "dlna")
	defer observability.DecPlaybackStream(method, "dlna")

	writer.Header().Set("Content-Type", "video/mp4")
	writer.Header().Set("Accept-Ranges", "bytes")
	counter := &countingResponseWriter{ResponseWriter: writer}
	http.ServeContent(counter, request, info.Name(), info.ModTime(), file)
	observability.AddPlaybackStreamBytes(method, "dlna", counter.n)
}

func (h *StreamHandler) serveMPEGTS(
	writer http.ResponseWriter,
	request *http.Request,
	mediaPath string,
	reencode bool,
) {
	abs, err := h.Media.FilePath(mediaPath)
	if err != nil {
		http.Error(writer, "not found", http.StatusNotFound)

		return
	}

	args := []string{"-hide_banner", "-loglevel", "error", "-i", abs}
	if reencode {
		args = append(args,
			"-map", "0:v:0", "-map", "0:a:0?",
			"-c:v", "libx264", "-preset", "veryfast", "-crf", "23",
			"-c:a", "aac", "-ac", "2",
			"-f", "mpegts", "pipe:1",
		)
	} else {
		args = append(args,
			"-map", "0:v:0", "-map", "0:a:0?",
			"-c", "copy",
			"-f", "mpegts", "pipe:1",
		)
	}

	cmd := exec.CommandContext(request.Context(), "ffmpeg", args...) //nolint:gosec // abs from mediafs
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		http.Error(writer, "stream failed", http.StatusInternalServerError)

		return
	}
	cmd.Stderr = io.Discard

	err = cmd.Start()
	if err != nil {
		http.Error(writer, "stream failed", http.StatusInternalServerError)

		return
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	writer.Header().Set("Content-Type", "video/mpeg")
	writer.WriteHeader(http.StatusOK)

	_, copyErr := io.Copy(writer, stdout)
	if copyErr != nil {
		slog.Debug("dlna mpegts copy ended", "err", copyErr, "path", mediaPath)
	}
}

func pathUnescapeKeepSlash(rawPath string) string {
	parts := strings.Split(rawPath, "/")
	for i, part := range parts {
		decoded, err := url.PathUnescape(part)
		if err != nil {
			return rawPath
		}
		parts[i] = decoded
	}

	return strings.Join(parts, "/")
}

type countingResponseWriter struct {
	http.ResponseWriter

	n int64
}

func (w *countingResponseWriter) Write(p []byte) (int, error) {
	written, err := w.ResponseWriter.Write(p)
	w.n += int64(written)
	if err != nil {
		return written, fmt.Errorf("write dlna stream: %w", err)
	}

	return written, nil
}
