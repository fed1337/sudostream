package provider

import (
	"context"
	"errors"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

type memoryStore struct {
	rows map[string]Settings
}

func newMemoryStore() *memoryStore {
	return &memoryStore{rows: map[string]Settings{}}
}

func (m *memoryStore) GetSettings(_ context.Context, libraryID string) (Settings, error) {
	row, ok := m.rows[libraryID]
	if !ok {
		return Settings{}, ErrNotFound
	}

	return row, nil
}

func (m *memoryStore) UpsertSettings(_ context.Context, settings Settings) (Settings, error) {
	m.rows[settings.LibraryID] = settings

	return settings, nil
}

func TestService_GetSettings_DefaultsWhenUnset(t *testing.T) {
	t.Parallel()

	allure.Test(t, "unconfigured library returns all-empty defaults", func(a *allure.Context) {
		t := a.T()
		svc := NewService(newMemoryStore(), NewRegistry())

		settings, err := svc.GetSettings(context.Background(), "lib-1")
		if err != nil {
			t.Fatalf("get settings: %v", err)
		}

		if settings.MetadataProvider != nil || settings.PosterProvider != nil ||
			settings.SubtitleProvider != nil {
			t.Fatalf("expected all slots empty, got %+v", settings)
		}
		if settings.MetadataApplyMode != ApplyModeFillMissing {
			t.Fatalf("expected default apply mode fill_missing, got %q", settings.MetadataApplyMode)
		}
		if settings.MetadataWriteTarget != WriteTargetDB {
			t.Fatalf("expected default write target db, got %q", settings.MetadataWriteTarget)
		}
		if len(settings.SubtitleLanguages) != 0 {
			t.Fatalf("expected no subtitle languages, got %v", settings.SubtitleLanguages)
		}
	})
}

func TestService_PatchSettings_RejectsInvalidProviders(t *testing.T) {
	t.Parallel()

	allure.Test(t, "patch rejects unregistered providers and bad enum values", func(a *allure.Context) {
		t := a.T()
		registry := NewRegistry()
		registry.RegisterMetadata(newFakeAdapter("anilist"))
		registry.RegisterPoster(newFakeAdapter("anilist"))
		svc := NewService(newMemoryStore(), registry)

		cases := []struct {
			name    string
			input   Settings
			wantErr error
		}{
			{
				name: "unregistered metadata provider",
				input: Settings{
					MetadataProvider:    new("tmdb"),
					MetadataApplyMode:   ApplyModeFillMissing,
					MetadataWriteTarget: WriteTargetDB,
				},
				wantErr: ErrInvalidMetadataProvider,
			},
			{
				name: "unregistered poster provider",
				input: Settings{
					PosterProvider:      new("tmdb"),
					MetadataApplyMode:   ApplyModeFillMissing,
					MetadataWriteTarget: WriteTargetDB,
				},
				wantErr: ErrInvalidPosterProvider,
			},
			{
				name: "unregistered subtitle provider (none shipped in v1)",
				input: Settings{
					SubtitleProvider:    new("opensubtitles"),
					SubtitleLanguages:   []string{"en"},
					MetadataApplyMode:   ApplyModeFillMissing,
					MetadataWriteTarget: WriteTargetDB,
				},
				wantErr: ErrInvalidSubtitleProvider,
			},
			{
				name: "invalid apply mode",
				input: Settings{
					MetadataApplyMode:   "delete_everything",
					MetadataWriteTarget: WriteTargetDB,
				},
				wantErr: ErrInvalidApplyMode,
			},
			{
				name: "invalid write target",
				input: Settings{
					MetadataApplyMode:   ApplyModeFillMissing,
					MetadataWriteTarget: "ftp",
				},
				wantErr: ErrInvalidWriteTarget,
			},
			{
				name: "metadata provider valid but apply mode missing",
				input: Settings{
					MetadataProvider:    new("anilist"),
					MetadataApplyMode:   "",
					MetadataWriteTarget: WriteTargetDB,
				},
				wantErr: ErrInvalidApplyMode,
			},
		}

		for _, tc := range cases {
			_, err := svc.PatchSettings(context.Background(), "lib-1", tc.input)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("%s: want %v, got %v", tc.name, tc.wantErr, err)
			}
		}
	})
}

func TestService_PatchSettings_SubtitleLanguageValidation(t *testing.T) {
	t.Parallel()

	allure.Test(t, "subtitle languages require ISO 639-1 alpha-2 and a provider", func(a *allure.Context) {
		t := a.T()
		registry := NewRegistry()
		registry.RegisterSubtitle(newFakeAdapter("opensubtitles"))
		svc := NewService(newMemoryStore(), registry)

		_, err := svc.PatchSettings(context.Background(), "lib-1", Settings{
			SubtitleProvider:    new("opensubtitles"),
			SubtitleLanguages:   nil,
			MetadataApplyMode:   ApplyModeFillMissing,
			MetadataWriteTarget: WriteTargetDB,
		})
		if !errors.Is(err, ErrSubtitleLanguagesRequired) {
			t.Fatalf("want ErrSubtitleLanguagesRequired, got %v", err)
		}

		_, err = svc.PatchSettings(context.Background(), "lib-1", Settings{
			SubtitleProvider:    new("opensubtitles"),
			SubtitleLanguages:   []string{"eng"},
			MetadataApplyMode:   ApplyModeFillMissing,
			MetadataWriteTarget: WriteTargetDB,
		})
		if !errors.Is(err, ErrInvalidSubtitleLanguage) {
			t.Fatalf("want ErrInvalidSubtitleLanguage, got %v", err)
		}

		saved, err := svc.PatchSettings(context.Background(), "lib-1", Settings{
			SubtitleProvider:    new("opensubtitles"),
			SubtitleLanguages:   []string{"EN", "ja", "en"},
			MetadataApplyMode:   ApplyModeFillMissing,
			MetadataWriteTarget: WriteTargetDB,
		})
		if err != nil {
			t.Fatalf("patch: %v", err)
		}
		if len(saved.SubtitleLanguages) != 2 {
			t.Fatalf("want deduped+normalized 2 langs, got %v", saved.SubtitleLanguages)
		}
	})
}

func TestService_PatchSettings_ClearingSubtitleProvider(t *testing.T) {
	t.Parallel()

	allure.Test(t, "clearing subtitle provider forces languages to empty", func(a *allure.Context) {
		t := a.T()
		svc := NewService(newMemoryStore(), NewRegistry())

		saved, err := svc.PatchSettings(context.Background(), "lib-1", Settings{
			SubtitleProvider:    nil,
			SubtitleLanguages:   []string{"en", "ru"},
			MetadataApplyMode:   ApplyModeFillMissing,
			MetadataWriteTarget: WriteTargetDB,
		})
		if err != nil {
			t.Fatalf("patch: %v", err)
		}
		if len(saved.SubtitleLanguages) != 0 {
			t.Fatalf("want languages cleared when no subtitle provider, got %v", saved.SubtitleLanguages)
		}
	})
}

func TestService_PatchSettings_RoundTrip(t *testing.T) {
	t.Parallel()

	allure.Test(t, "valid settings round-trip through the store", func(a *allure.Context) {
		t := a.T()
		registry := NewRegistry()
		registry.RegisterMetadata(newFakeAdapter("anilist"))
		registry.RegisterPoster(newFakeAdapter("anilist"))
		svc := NewService(newMemoryStore(), registry)

		saved, err := svc.PatchSettings(context.Background(), "lib-1", Settings{
			MetadataProvider:          new("anilist"),
			PosterProvider:            new("anilist"),
			AllowOverrideUserMetadata: true,
			MetadataApplyMode:         ApplyModeFullRewrite,
			MetadataWriteTarget:       WriteTargetFile,
		})
		if err != nil {
			t.Fatalf("patch: %v", err)
		}
		if saved.LibraryID != "lib-1" {
			t.Fatalf("want libraryId set from path param, got %q", saved.LibraryID)
		}

		fetched, err := svc.GetSettings(context.Background(), "lib-1")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if fetched.MetadataApplyMode != ApplyModeFullRewrite ||
			fetched.MetadataWriteTarget != WriteTargetFile {
			t.Fatalf("round-trip mismatch: %+v", fetched)
		}
	})
}

func TestService_NewServiceDefaultsRegistryAndExposesIt(t *testing.T) {
	t.Parallel()

	allure.Test(t, "NewService defaults a nil registry and Registry() exposes it", func(a *allure.Context) {
		t := a.T()
		svc := NewService(newMemoryStore(), nil)

		if svc.Registry() == nil {
			t.Fatal("want non-nil registry when constructed with nil")
		}
		if svc.Registry().SupportsMetadata("anilist") {
			t.Fatal("want empty default registry")
		}
	})
}

func TestRegistry_RegisterAndQuery(t *testing.T) {
	t.Parallel()

	allure.Test(t, "registry tracks keys per category independently", func(a *allure.Context) {
		t := a.T()
		reg := NewRegistry()
		reg.RegisterMetadata(newFakeAdapter("anilist"))
		reg.RegisterMetadata(newFakeAdapter("anilist"))
		reg.RegisterPoster(newFakeAdapter("anilist"))

		if !reg.SupportsMetadata("anilist") {
			t.Fatal("want anilist registered for metadata")
		}
		if reg.SupportsSubtitle("anilist") {
			t.Fatal("anilist must not be valid for subtitle slot")
		}
		if len(reg.Metadata()) != 1 {
			t.Fatalf("want dedup on repeat register, got %v", reg.Metadata())
		}
		if len(reg.Subtitle()) != 0 {
			t.Fatalf("want empty subtitle registry in v1, got %v", reg.Subtitle())
		}
	})
}
