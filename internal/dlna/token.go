package dlna

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// StreamMode selects how the DLNA stream handler delivers bytes.
type StreamMode string

const (
	// StreamDirect serves the original file with Range.
	StreamDirect StreamMode = "direct"
	// StreamRemux pipes ffmpeg -c copy MPEG-TS.
	StreamRemux StreamMode = "remux"
	// StreamTranscode pipes ffmpeg re-encode MPEG-TS.
	StreamTranscode StreamMode = "transcode"
)

// SignStream mints a query string for progressive play (TTL 24h).
func SignStream(secret []byte, userID, mediaPath string, mode StreamMode, now time.Time) string {
	exp := now.UTC().Add(streamTokenTTLSec * time.Second).Unix()
	path := strings.TrimPrefix(mediaPath, "/")
	sig := streamSignature(secret, userID, path, mode, exp)
	values := url.Values{}
	values.Set("uid", userID)
	values.Set("exp", strconv.FormatInt(exp, 10))
	values.Set("m", string(mode))
	values.Set("sig", sig)

	return values.Encode()
}

// VerifyStream validates signed query params for a media path.
func VerifyStream(
	secret []byte,
	userID, mediaPath, modeRaw, expRaw, sig string,
	now time.Time,
) (StreamMode, error) {
	if len(secret) == 0 || userID == "" || sig == "" || expRaw == "" {
		return "", ErrInvalidToken
	}

	exp, err := strconv.ParseInt(expRaw, 10, 64)
	if err != nil || exp < now.UTC().Unix() {
		return "", ErrInvalidToken
	}

	mode := StreamMode(strings.TrimSpace(modeRaw))
	switch mode {
	case StreamDirect, StreamRemux, StreamTranscode:
	default:
		return "", ErrInvalidToken
	}

	path := strings.TrimPrefix(mediaPath, "/")
	expected := streamSignature(secret, userID, path, mode, exp)
	if !hmac.Equal([]byte(expected), []byte(sig)) {
		return "", ErrInvalidToken
	}

	return mode, nil
}

func streamSignature(secret []byte, userID, path string, mode StreamMode, exp int64) string {
	payload := fmt.Sprintf("%s|%s|%s|%d", userID, path, mode, exp)
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(payload))

	return hex.EncodeToString(mac.Sum(nil))
}

// EncodeObjectID builds an opaque stable ContentDirectory object id.
func EncodeObjectID(parts ...string) string {
	joined := strings.Join(parts, "\x1f")

	return base64.RawURLEncoding.EncodeToString([]byte(joined))
}

// DecodeObjectID splits an opaque object id into parts.
func DecodeObjectID(id string) ([]string, error) {
	if id == "" || id == "0" {
		return []string{"0"}, nil
	}

	raw, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil {
		return nil, fmt.Errorf("decode object id: %w", err)
	}

	return strings.Split(string(raw), "\x1f"), nil
}
