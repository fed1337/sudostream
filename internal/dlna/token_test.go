package dlna

import (
	"errors"
	"net/url"
	"sudoStream/internal/playback"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestSignAndVerifyStream(t *testing.T) {
	t.Parallel()

	allure.Test(t, "signed stream tokens verify within TTL", func(a *allure.Context) {
		t := a.T()
		secret := []byte("test-secret-key-32-bytes-minimum!")
		now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		query := SignStream(secret, "user-1", "films/a.mp4", StreamDirect, now)
		values, err := url.ParseQuery(query)
		if err != nil {
			t.Fatal(err)
		}
		mode, err := VerifyStream(
			secret,
			values.Get("uid"),
			"films/a.mp4",
			values.Get("m"),
			values.Get("exp"),
			values.Get("sig"),
			now,
		)
		if err != nil || mode != StreamDirect {
			t.Fatalf("verify: mode=%q err=%v", mode, err)
		}

		_, err = VerifyStream(
			secret,
			values.Get("uid"),
			"films/a.mp4",
			values.Get("m"),
			values.Get("exp"),
			values.Get("sig"),
			now.Add(25*time.Hour),
		)
		if !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("expected expired token, got %v", err)
		}
	})
}

func TestObjectIDRoundTrip(t *testing.T) {
	t.Parallel()

	allure.Test(t, "object ids encode and decode", func(a *allure.Context) {
		t := a.T()
		id := EncodeObjectID("lib", "abc", "files")
		parts, err := DecodeObjectID(id)
		if err != nil {
			t.Fatal(err)
		}
		if len(parts) != 3 || parts[0] != "lib" || parts[1] != "abc" || parts[2] != "files" {
			t.Fatalf("parts=%v", parts)
		}
		root, err := DecodeObjectID("0")
		if err != nil || len(root) != 1 || root[0] != "0" {
			t.Fatalf("root=%v err=%v", root, err)
		}
	})
}

func TestStreamModeForDecision(t *testing.T) {
	t.Parallel()

	allure.Test(t, "FI-8 decisions map to DLNA stream modes", func(a *allure.Context) {
		t := a.T()
		if StreamModeForDecision(playback.MethodDirectPlay) != StreamDirect {
			t.Fatal("direct")
		}
		if StreamModeForDecision(playback.MethodRemux) != StreamRemux {
			t.Fatal("remux")
		}
		if StreamModeForDecision(playback.MethodTranscode) != StreamTranscode {
			t.Fatal("transcode")
		}
	})
}

func TestNormalizeSettings_RequiresTVUser(t *testing.T) {
	t.Parallel()

	allure.Test(t, "enabled DLNA requires userId", func(a *allure.Context) {
		t := a.T()
		_, err := NormalizeSettings(Settings{Enabled: true})
		if !errors.Is(err, ErrTVUserRequired) {
			t.Fatalf("got %v", err)
		}
	})
}
