package usersub

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestService_PutGetReplaceExpire(t *testing.T) { //nolint:cyclop // multi-step scenario
	t.Parallel()

	allure.Test(t, "usersub put/get/replace/TTL", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		svc, err := NewService(filepath.Join(root, "user-subs"))
		if err != nil {
			t.Fatalf("NewService: %v", err)
		}

		track, err := svc.Put(
			"user-1",
			"shows/a.mkv",
			"en",
			"English",
			[]byte("WEBVTT\n\n00:00:00.000 --> 00:00:01.000\nHi\n"),
		)
		if err != nil {
			t.Fatalf("Put: %v", err)
		}
		if track.Lang != "en" || track.Label != "English" {
			t.Fatalf("track: %+v", track)
		}

		got, path, err := svc.Get("user-1", "shows/a.mkv")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.Lang != "en" {
			t.Fatalf("got: %+v", got)
		}
		raw, err := os.ReadFile(path) //nolint:gosec // test path
		if err != nil || !strings.Contains(string(raw), "Hi") {
			t.Fatalf("vtt body: %q err=%v", raw, err)
		}

		_, err = svc.Put(
			"user-1",
			"shows/a.mkv",
			"ru",
			"Русский",
			[]byte("WEBVTT\n\n00:00:00.000 --> 00:00:01.000\nПривет\n"),
		)
		if err != nil {
			t.Fatalf("replace: %v", err)
		}
		got, _, err = svc.Get("user-1", "shows/a.mkv")
		if err != nil || got.Lang != "ru" {
			t.Fatalf("after replace: %+v err=%v", got, err)
		}

		_, _, err = svc.Get("user-2", "shows/a.mkv")
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("other user: %v", err)
		}

		dir := svc.userDir("user-1")
		base := svc.baseName(normalizePath("shows/a.mkv"))
		metaPath := filepath.Join(dir, base+metaSuffix)
		expired := metaFile{
			RelPath:   "shows/a.mkv",
			Lang:      "ru",
			Label:     "Русский",
			ExpiresAt: time.Now().UTC().Add(-time.Minute),
		}
		rawMeta, err := json.Marshal(expired)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		err = os.WriteFile(metaPath, rawMeta, filePerm)
		if err != nil {
			t.Fatalf("write expired meta: %v", err)
		}
		_, _, err = svc.Get("user-1", "shows/a.mkv")
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("expired: %v", err)
		}
	})
}
