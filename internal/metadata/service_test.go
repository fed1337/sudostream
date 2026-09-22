package metadata

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sudoStream/internal/access"
	"sudoStream/internal/mediafs"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

const (
	testLibraryID   = "lib-1"
	testLibrarySlug = "series"
)

var testWebMHeader = []byte{
	0x1a, 0x45, 0xdf, 0xa3, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x10, 0x42, 0x82, 0x85, 0x77,
	0x65, 0x62, 0x6d, 0x00, 0x42, 0x87, 0x81, 0x02, 0x42, 0x85, 0x81, 0x02, 0x18, 0x53, 0x80, 0x67,
	0x01, 0x00, 0x00, 0x00, 0x00, 0x21, 0x5a, 0xf8, 0x11, 0x4d, 0x9b, 0x74, 0x01, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x8c, 0x4d, 0xbb, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x12, 0x53, 0xab,
	0x84, 0x15, 0x49, 0xa9, 0x66, 0x53, 0xac, 0x88, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x98,
}

type memoryRepository struct {
	rows      map[string]Row
	getErr    error
	updateErr error
}

func (m *memoryRepository) Get(_ context.Context, libraryID, relPath string) (Row, error) {
	if m.getErr != nil {
		return Row{}, m.getErr
	}

	row, ok := m.rows[m.key(libraryID, relPath)]
	if !ok {
		return Row{}, ErrNotFound
	}

	return row, nil
}

func (m *memoryRepository) UpdateOverride(
	_ context.Context,
	libraryID, relPath string,
	override StoredOverride,
	updatedAt time.Time,
	overriddenBy string,
) error {
	if m.updateErr != nil {
		return m.updateErr
	}

	key := m.key(libraryID, relPath)
	row := m.rows[key]
	row.LibraryID = libraryID
	row.RelPath = relPath
	row.Override = override
	row.OverrideAt = &updatedAt
	row.OverriddenBy = &overriddenBy
	m.rows[key] = row

	return nil
}

func (m *memoryRepository) UpsertOriginal(
	_ context.Context,
	libraryID, relPath string,
	original VideoFields,
	fileMtime time.Time,
	fileSize int64,
	probedAt time.Time,
) error {
	if m.updateErr != nil {
		return m.updateErr
	}

	key := m.key(libraryID, relPath)
	row := m.rows[key]
	row.LibraryID = libraryID
	row.RelPath = relPath
	row.Original = original
	row.FileMtime = &fileMtime
	row.FileSize = &fileSize
	row.ProbedAt = &probedAt
	m.rows[key] = row

	return nil
}

func (m *memoryRepository) DeletePath(_ context.Context, libraryID, relPath string) error {
	delete(m.rows, m.key(libraryID, relPath))

	return nil
}

func (m *memoryRepository) Search(
	_ context.Context,
	libraryIDs []string,
	tokens []string,
	_ string,
	limit, offset int,
) ([]SearchRow, int, error) {
	allowed := make(map[string]struct{}, len(libraryIDs))
	for _, libraryID := range libraryIDs {
		allowed[libraryID] = struct{}{}
	}

	matches := make([]SearchRow, 0)
	for _, row := range m.rows {
		if _, ok := allowed[row.LibraryID]; !ok {
			continue
		}

		document := BuildSearchDocument(row.RelPath, row.Original, row.Override.VideoFields)
		if !documentHasAllTokens(document, tokens) {
			continue
		}

		matches = append(matches, SearchRow{LibraryID: row.LibraryID, RelPath: row.RelPath})
	}

	total := len(matches)
	if offset > total {
		return nil, total, nil
	}
	end := min(offset+limit, total)
	if limit <= 0 {
		end = total
	}

	return matches[offset:end], total, nil
}

func (m *memoryRepository) ListActiveIndexedPaths(
	_ context.Context,
	libraryID string,
) ([]string, error) {
	paths := make([]string, 0)
	for _, row := range m.rows {
		if row.LibraryID != libraryID {
			continue
		}
		if row.RelPath == ".trash" || strings.HasPrefix(row.RelPath, ".trash/") {
			continue
		}
		paths = append(paths, row.RelPath)
	}

	return paths, nil
}

func documentHasAllTokens(document string, tokens []string) bool {
	haystack := strings.ToLower(document)
	for _, token := range tokens {
		if !strings.Contains(haystack, strings.ToLower(token)) {
			return false
		}
	}

	return true
}

func (m *memoryRepository) key(libraryID, relPath string) string {
	return libraryID + "|" + relPath
}

type memoryAccess struct {
	libraries []access.Library
}

func (m *memoryAccess) ListLibraries(context.Context) ([]access.Library, error) {
	return m.libraries, nil
}

var errBoom = errors.New("boom")

type errAccess struct {
	err error
}

func (m errAccess) ListLibraries(context.Context) ([]access.Library, error) {
	return nil, m.err
}

func seriesCatalog() *memoryAccess {
	return &memoryAccess{
		libraries: []access.Library{{
			ID: testLibraryID, RelPath: testLibrarySlug, Type: access.LibraryTypeSeries,
			Roots: []string{testLibrarySlug, "friends"},
		}},
	}
}

func TestService_Search_SceneReleaseFilename(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"search finds scene-release filename friends even when JSON title is empty",
		func(a *allure.Context) {
			t := a.T()
			relPath := "friends/[linuxisos.ru].friends.s01.e01.mkv"
			repo := &memoryRepository{rows: map[string]Row{
				testLibraryID + "|" + relPath: {
					LibraryID: testLibraryID,
					RelPath:   relPath,
				},
			}}
			svc := NewService(nil, seriesCatalog(), repo, nil)

			hits, total, err := svc.Search(
				context.Background(),
				[]string{testLibraryID},
				"friends",
				28,
				0,
			)
			if err != nil {
				t.Fatalf("search: %v", err)
			}
			if total != 1 || len(hits) != 1 {
				t.Fatalf("hits=%d total=%d", len(hits), total)
			}
			if hits[0].Path != relPath {
				t.Fatalf("path=%q", hits[0].Path)
			}
			if hits[0].ShowKey == "" {
				t.Fatal("expected series showKey")
			}
		},
	)
}

func TestService_GetAndPatchOverride(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"metadata service merges override and returns display name",
		func(a *allure.Context) {
			t := a.T()
			title := "Embedded"
			show := "Demo"
			season := 1
			episode := 1

			repo := &memoryRepository{rows: map[string]Row{
				"lib-1|series/Demo/S01E01.mkv": {
					LibraryID: testLibraryID,
					RelPath:   "series/Demo/S01E01.mkv",
					Original: VideoFields{
						Title:   &title,
						Show:    &show,
						Season:  &season,
						Episode: &episode,
					},
				},
			}}

			accessService := &memoryAccess{
				libraries: []access.Library{{
					ID:      testLibraryID,
					Slug:    testLibrarySlug,
					RelPath: testLibrarySlug,
					Name:    testLibrarySlug,
					Type:    access.LibraryTypeSeries,
				}},
			}

			service := &Service{
				media:   nil,
				access:  accessService,
				store:   repo,
				indexer: nil,
			}

			response, err := service.Get(context.Background(), "series/Demo/S01E01.mkv")
			if err != nil {
				t.Fatalf("get metadata: %v", err)
			}

			if response.DisplayName != "Demo — S01E01" {
				t.Fatalf("unexpected display name: %q", response.DisplayName)
			}

			overrideTitle := "Override Pilot"
			patched, err := service.Patch(
				context.Background(),
				"series/Demo/S01E01.mkv",
				PatchRequest{
					Target: PatchTargetOverride,
					Fields: map[string]*jsonValue{
						fieldTitle: {raw: overrideTitle},
					},
				},
				"user-1",
			)
			if err != nil {
				t.Fatalf("patch metadata: %v", err)
			}

			if patched.Effective.Title == nil || *patched.Effective.Title != overrideTitle {
				t.Fatalf(
					"expected override title in effective fields, got %#v",
					patched.Effective.Title,
				)
			}
		},
	)
}

func TestService_PatchFileTargetRejected(t *testing.T) {
	t.Parallel()

	allure.Test(t, "file target is rejected before phase 5", func(a *allure.Context) {
		t := a.T()
		service := &Service{
			access: &memoryAccess{
				libraries: []access.Library{{
					ID: testLibraryID, RelPath: testLibrarySlug, Type: access.LibraryTypeSeries,
				}},
			},
			store: &memoryRepository{rows: map[string]Row{}},
		}

		_, err := service.Patch(context.Background(), "series/Demo/S01E01.mkv", PatchRequest{
			Target: PatchTargetFile,
			Fields: map[string]*jsonValue{},
		}, "user-1")
		if !errors.Is(err, ErrFileTargetUnsupported) {
			t.Fatalf("expected ErrFileTargetUnsupported, got %v", err)
		}
	})
}

func newSeriesService() *Service {
	return &Service{
		access: &memoryAccess{
			libraries: []access.Library{{
				ID: testLibraryID, RelPath: testLibrarySlug, Type: access.LibraryTypeSeries,
			}},
		},
		store: &memoryRepository{rows: map[string]Row{}},
	}
}

func TestService_GetRejectsNonVideoAndUnknownLibrary(t *testing.T) {
	t.Parallel()

	allure.Test(t, "get returns typed errors for bad paths", func(a *allure.Context) {
		t := a.T()
		service := newSeriesService()

		_, err := service.Get(context.Background(), "series/notes.txt")
		if !errors.Is(err, ErrNotVideo) {
			t.Fatalf("expected ErrNotVideo, got %v", err)
		}

		_, err = service.Get(context.Background(), "unknown/clip.mkv")
		if !errors.Is(err, ErrUnknownLibrary) {
			t.Fatalf("expected ErrUnknownLibrary, got %v", err)
		}

		_, err = service.Get(context.Background(), "")
		if err == nil {
			t.Fatal("expected error for empty path")
		}
	})
}

func TestService_GetAcceptsExtensionlessVideo(t *testing.T) {
	t.Parallel()

	allure.Test(t, "extensionless webm is accepted via MIME sniffing", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()
		mediaDir := filepath.Join(root, testLibrarySlug, "Demo")
		err := os.MkdirAll(mediaDir, 0o750)
		if err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		// Minimal WebM EBML header; enough for http.DetectContentType → video/webm.
		mediaPath := filepath.Join(mediaDir, "SHORTNAME")
		err = os.WriteFile(mediaPath, testWebMHeader, 0o600)
		if err != nil {
			t.Fatalf("write fixture: %v", err)
		}

		media, err := mediafs.New(root)
		if err != nil {
			t.Fatalf("mediafs: %v", err)
		}

		service := &Service{
			media: media,
			access: &memoryAccess{
				libraries: []access.Library{{
					ID: testLibraryID, RelPath: testLibrarySlug, Type: access.LibraryTypeSeries,
				}},
			},
			store: &memoryRepository{rows: map[string]Row{}},
		}

		_, err = service.Get(context.Background(), testLibrarySlug+"/Demo/SHORTNAME")
		if err != nil {
			t.Fatalf("expected extensionless webm metadata, got %v", err)
		}
	})
}

func TestService_GetMissingRowReturnsHeuristicResponse(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"get without stored row falls back to filename heuristics",
		func(a *allure.Context) {
			t := a.T()
			service := newSeriesService()

			response, err := service.Get(context.Background(), "series/Demo/S02E04.mkv")
			if err != nil {
				t.Fatalf("get metadata: %v", err)
			}
			if response.ProbedAt != nil {
				t.Fatalf("expected nil probed_at, got %#v", response.ProbedAt)
			}
			if response.Effective.Season == nil || *response.Effective.Season != 2 {
				t.Fatalf("expected heuristic season, got %#v", response.Effective.Season)
			}
		},
	)
}

func TestService_PatchRejectsInvalidTargets(t *testing.T) {
	t.Parallel()

	allure.Test(t, "patch rejects empty and unknown targets", func(a *allure.Context) {
		t := a.T()
		service := newSeriesService()

		for _, target := range []PatchTarget{"", "bogus"} {
			_, err := service.Patch(context.Background(), "series/Demo/S01E01.mkv", PatchRequest{
				Target: target,
				Fields: map[string]*jsonValue{},
			}, "user-1")
			if !errors.Is(err, ErrInvalidTarget) {
				t.Fatalf("target %q: expected ErrInvalidTarget, got %v", target, err)
			}
		}
	})
}

func TestService_ErrorPaths(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"store and access failures surface as wrapped or typed errors",
		func(a *allure.Context) {
			t := a.T()
			ctx := context.Background()
			const videoPath = "series/Demo/S01E01.mkv"

			getErrService := &Service{
				access: seriesCatalog(),
				store:  &memoryRepository{rows: map[string]Row{}, getErr: errBoom},
			}
			_, err := getErrService.Get(ctx, videoPath)
			if err == nil || errors.Is(err, ErrNotFound) {
				t.Fatalf("expected wrapped load error, got %v", err)
			}

			listErrService := &Service{
				access: errAccess{err: errBoom},
				store:  &memoryRepository{rows: map[string]Row{}},
			}
			_, err = listErrService.Get(ctx, videoPath)
			if err == nil {
				t.Fatal("expected list libraries error from Get")
			}
			_, err = listErrService.Patch(ctx, videoPath, PatchRequest{
				Target: PatchTargetOverride,
				Fields: map[string]*jsonValue{fieldTitle: {raw: "X"}},
			}, "user-1")
			if err == nil {
				t.Fatal("expected list libraries error from Patch")
			}

			updateErrService := &Service{
				access: seriesCatalog(),
				store:  &memoryRepository{rows: map[string]Row{}, updateErr: errBoom},
			}
			_, err = updateErrService.Patch(ctx, videoPath, PatchRequest{
				Target: PatchTargetOverride,
				Fields: map[string]*jsonValue{fieldTitle: {raw: "X"}},
			}, "user-1")
			if err == nil {
				t.Fatal("expected update override error")
			}

			invalidValueService := newSeriesService()
			_, err = invalidValueService.Patch(ctx, videoPath, PatchRequest{
				Target: PatchTargetOverride,
				Fields: map[string]*jsonValue{fieldSeason: {raw: "not-an-int"}},
			}, "user-1")
			if !errors.Is(err, ErrInvalidPatchValue) {
				t.Fatalf("expected ErrInvalidPatchValue, got %v", err)
			}
		},
	)
}

func TestService_NilReceiverAndIndexerGuards(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"nil service reports unavailable and Indexer guards nil",
		func(a *allure.Context) {
			t := a.T()
			var service *Service

			_, err := service.Get(context.Background(), "series/Demo/S01E01.mkv")
			if !errors.Is(err, ErrServiceUnavailable) {
				t.Fatalf("expected ErrServiceUnavailable from Get, got %v", err)
			}

			_, err = service.Patch(context.Background(), "series/Demo/S01E01.mkv", PatchRequest{
				Target: PatchTargetOverride,
				Fields: map[string]*jsonValue{},
			}, "user-1")
			if !errors.Is(err, ErrServiceUnavailable) {
				t.Fatalf("expected ErrServiceUnavailable from Patch, got %v", err)
			}

			if service.Indexer() != nil {
				t.Fatal("expected nil Indexer for nil service")
			}

			configured := newSeriesService()
			if configured.Indexer() != nil {
				t.Fatal("expected nil Indexer when none configured")
			}
		},
	)
}

func newSeriesServiceWithMedia(t *testing.T) (*Service, string) {
	t.Helper()

	root := t.TempDir()
	mediaDir := filepath.Join(root, testLibrarySlug, "Demo")
	err := os.MkdirAll(mediaDir, 0o750)
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	mediaPath := filepath.Join(mediaDir, "S01E01.mkv")
	err = os.WriteFile(mediaPath, testWebMHeader, 0o600)
	if err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	media, err := mediafs.New(root)
	if err != nil {
		t.Fatalf("mediafs: %v", err)
	}

	return &Service{
		media:  media,
		access: seriesCatalog(),
		store:  &memoryRepository{rows: map[string]Row{}},
	}, testLibrarySlug + "/Demo/S01E01.mkv"
}

const testFilePatchTitle = "File Title"

func TestService_PatchFileTarget_HappyPath(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"patch file target writes tags and re-probes original fields",
		func(a *allure.Context) {
			t := a.T()
			service, relPath := newSeriesServiceWithMedia(t)
			probedTitle := testFilePatchTitle

			service.writeTags = func(_ context.Context, _ string, tags map[string]string) error {
				if tags["title"] != testFilePatchTitle {
					t.Fatalf("write tags: got %#v", tags)
				}

				return nil
			}
			service.probe = func(context.Context, string) (VideoFields, error) {
				return VideoFields{Title: &probedTitle}, nil
			}

			patched, err := service.Patch(context.Background(), relPath, PatchRequest{
				Target: PatchTargetFile,
				Fields: map[string]*jsonValue{fieldTitle: {raw: testFilePatchTitle}},
			}, "user-1")
			if err != nil {
				t.Fatalf("patch file: %v", err)
			}
			if patched.Effective.Title == nil || *patched.Effective.Title != probedTitle {
				t.Fatalf("effective title: got %#v", patched.Effective.Title)
			}
			if patched.ProbedAt == nil {
				t.Fatal("expected probed_at after file patch")
			}
			if patched.OverriddenBy == nil ||
				*patched.OverriddenBy != FormatUserOverriddenBy("user-1") {
				t.Fatalf("expected overridden_by user, got %#v", patched.OverriddenBy)
			}
			if patched.Source.Override.Title != nil {
				t.Fatalf(
					"expected override title cleared after file write, got %#v",
					patched.Source.Override.Title,
				)
			}
		},
	)
}

func TestService_PatchFileTarget_NullKeepsOverride(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"null file title clears container tag but keeps override",
		func(a *allure.Context) {
			t := a.T()
			service, relPath := newSeriesServiceWithMedia(t)
			overrideTitle := "FromOverride"
			_, err := service.Patch(context.Background(), relPath, PatchRequest{
				Target: PatchTargetOverride,
				Fields: map[string]*jsonValue{fieldTitle: {raw: overrideTitle}},
			}, "user-1")
			if err != nil {
				t.Fatalf("seed override: %v", err)
			}

			service.writeTags = func(_ context.Context, _ string, tags map[string]string) error {
				if tags["title"] != "" {
					t.Fatalf("expected empty title tag, got %#v", tags)
				}

				return nil
			}
			service.probe = func(context.Context, string) (VideoFields, error) {
				return VideoFields{}, nil
			}

			patched, err := service.Patch(context.Background(), relPath, PatchRequest{
				Target: PatchTargetFile,
				Fields: map[string]*jsonValue{fieldTitle: nil},
			}, "user-1")
			if err != nil {
				t.Fatalf("patch file null: %v", err)
			}
			if patched.Source.Original.Title != nil {
				t.Fatalf("expected empty original title, got %#v", patched.Source.Original.Title)
			}
			if patched.Source.Override.Title == nil ||
				*patched.Source.Override.Title != overrideTitle {
				t.Fatalf("expected override kept, got %#v", patched.Source.Override.Title)
			}
			if patched.Effective.Title == nil || *patched.Effective.Title != overrideTitle {
				t.Fatalf("expected effective from override, got %#v", patched.Effective.Title)
			}
		},
	)
}

func TestService_PatchFileTarget_WriteError(t *testing.T) {
	t.Parallel()

	allure.Test(t, "patch file target surfaces writeTags failures", func(a *allure.Context) {
		t := a.T()
		service, relPath := newSeriesServiceWithMedia(t)
		service.writeTags = func(context.Context, string, map[string]string) error {
			return errBoom
		}

		_, err := service.Patch(context.Background(), relPath, PatchRequest{
			Target: PatchTargetFile,
			Fields: map[string]*jsonValue{fieldTitle: {raw: "X"}},
		}, "user-1")
		if err == nil || !errors.Is(err, errBoom) {
			t.Fatalf("expected write error, got %v", err)
		}
	})
}

func TestService_PatchFileTarget_ProbeError(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"patch file target surfaces probe failures after write",
		func(a *allure.Context) {
			t := a.T()
			service, relPath := newSeriesServiceWithMedia(t)
			service.writeTags = func(context.Context, string, map[string]string) error {
				return nil
			}
			service.probe = func(context.Context, string) (VideoFields, error) {
				return VideoFields{}, errBoom
			}

			_, err := service.Patch(context.Background(), relPath, PatchRequest{
				Target: PatchTargetFile,
				Fields: map[string]*jsonValue{fieldTitle: {raw: "X"}},
			}, "user-1")
			if err == nil {
				t.Fatal("expected probe error")
			}
		},
	)
}

func TestService_PatchFileTarget_InvalidFields(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"patch file target validates media type and patch values",
		func(a *allure.Context) {
			t := a.T()
			service, relPath := newSeriesServiceWithMedia(t)

			_, err := service.Patch(context.Background(), "series/notes.txt", PatchRequest{
				Target: PatchTargetFile,
				Fields: map[string]*jsonValue{fieldTitle: {raw: "X"}},
			}, "user-1")
			if !errors.Is(err, ErrNotVideo) {
				t.Fatalf("expected ErrNotVideo, got %v", err)
			}

			_, err = service.Patch(context.Background(), "unknown/clip.mkv", PatchRequest{
				Target: PatchTargetFile,
				Fields: map[string]*jsonValue{fieldTitle: {raw: "X"}},
			}, "user-1")
			if !errors.Is(err, ErrUnknownLibrary) {
				t.Fatalf("expected ErrUnknownLibrary, got %v", err)
			}

			_, err = service.Patch(context.Background(), relPath, PatchRequest{
				Target: PatchTargetFile,
				Fields: map[string]*jsonValue{fieldSeason: {raw: "not-an-int"}},
			}, "user-1")
			if !errors.Is(err, ErrInvalidPatchValue) {
				t.Fatalf("expected ErrInvalidPatchValue, got %v", err)
			}
		},
	)
}
