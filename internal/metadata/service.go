package metadata

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sudoStream/internal/access"
	"sudoStream/internal/mediafs"
	"sudoStream/internal/mediahash"
	"sudoStream/internal/observability"
	"sync"
	"time"
)

// IndexStore persists probed metadata rows.
type IndexStore interface {
	Get(ctx context.Context, libraryID, relPath string) (Row, error)
	UpsertOriginal(
		ctx context.Context,
		libraryID, relPath string,
		original VideoFields,
		fileMtime time.Time,
		fileSize int64,
		probedAt time.Time,
	) error
	ListIndexedPaths(ctx context.Context, libraryID string) ([]string, error)
	DeletePath(ctx context.Context, libraryID, relPath string) error
	NeedsProbe(
		ctx context.Context,
		libraryID, relPath string,
		fileMtime time.Time,
		fileSize int64,
	) (bool, error)
}

// Indexer walks libraries and caches ffprobe results.
type Indexer struct {
	media    *mediafs.Service
	store    IndexStore
	frozen   FrozenPaths
	probe    func(ctx context.Context, absPath string) (VideoFields, error)
	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// FrozenPaths lists original rel paths sitting in the recycle bin.
type FrozenPaths interface {
	OriginalPrefixes(ctx context.Context) ([]string, error)
}

// NewIndexer constructs a metadata indexer.
func NewIndexer(media *mediafs.Service, store IndexStore) *Indexer {
	return &Indexer{
		media:  media,
		store:  store,
		stopCh: make(chan struct{}),
	}
}

// SetFrozenPaths skips recycle-bin originals during stale-row prune.
func (i *Indexer) SetFrozenPaths(frozen FrozenPaths) {
	if i == nil {
		return
	}
	i.frozen = frozen
}

// Shutdown cancels in-flight indexing work and waits for background goroutines.
func (i *Indexer) Shutdown() {
	if i == nil || i.stopCh == nil {
		return
	}

	i.stopOnce.Do(func() {
		close(i.stopCh)
	})
	i.wg.Wait()
}

// IndexLibraryAsync probes changed video files for one library in the background.
func (i *Indexer) IndexLibraryAsync(_ context.Context, library access.Library) {
	if i == nil || i.media == nil || i.store == nil || library.ID == "" {
		return
	}

	select {
	case <-i.stopCh:
		return
	default:
	}

	//nolint:contextcheck // background indexing uses service-owned shutdown context
	i.wg.Go(func() {
		ctx, cancel := i.backgroundContext()
		defer cancel()
		i.indexLibrary(ctx, library)
	})
}

// IndexLibrariesAsync probes changed video files for each library in the background.
func (i *Indexer) IndexLibrariesAsync(_ context.Context, libraries []access.Library) {
	if i == nil || i.media == nil || i.store == nil || len(libraries) == 0 {
		return
	}

	select {
	case <-i.stopCh:
		return
	default:
	}

	//nolint:contextcheck // background indexing uses service-owned shutdown context
	i.wg.Go(func() {
		ctx, cancel := i.backgroundContext()
		defer cancel()

		for _, library := range libraries {
			if ctx.Err() != nil {
				return
			}

			i.indexLibrary(ctx, library)
		}
	})
}

// IndexLibrary probes changed video files for one library and returns files indexed.
func (i *Indexer) IndexLibrary(ctx context.Context, library access.Library) (int, error) {
	if i == nil || i.media == nil || i.store == nil || library.ID == "" {
		return 0, nil
	}

	if ctx.Err() != nil {
		return 0, fmt.Errorf("index library canceled: %w", ctx.Err())
	}

	count := i.indexLibrary(ctx, library)
	err := ctx.Err()
	if err != nil {
		return count, fmt.Errorf("index library canceled: %w", err)
	}

	return count, nil
}

func (i *Indexer) backgroundContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		select {
		case <-i.stopCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	return ctx, cancel
}

func (i *Indexer) indexLibrary(
	ctx context.Context,
	library access.Library,
) int {
	roots := library.RootPaths()
	if len(roots) == 0 {
		return 0
	}

	indexed := make(map[string]struct{})
	var filesIndexed int

	for _, root := range roots {
		count := i.indexRoot(ctx, library, root, indexed)
		filesIndexed += count
	}

	i.deleteStalePaths(ctx, library, indexed)

	slog.Info("metadata index completed",
		slog.String("action", "metadata.index"),
		slog.String("library_id", library.ID),
		slog.String("library_slug", library.Slug),
		slog.Int("files_indexed", filesIndexed),
	)

	return filesIndexed
}

func (i *Indexer) indexRoot( //nolint:cyclop,funlen // walk callback
	ctx context.Context,
	library access.Library,
	root string,
	indexed map[string]struct{},
) int {
	rootPath, err := i.media.DirPath(root)
	if err != nil {
		slog.Warn("metadata index skipped library root",
			slog.String("action", "metadata.index"),
			slog.String("library_id", library.ID),
			slog.String("library_slug", library.Slug),
			slog.String("root", root),
			slog.String("error", err.Error()),
		)

		return 0
	}

	var filesIndexed int

	walkErr := filepath.WalkDir(rootPath, func(absPath string, entry fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if err != nil {
			return err
		}

		if entry.IsDir() {
			if strings.HasPrefix(entry.Name(), ".") && absPath != rootPath {
				return filepath.SkipDir
			}

			return nil
		}

		if !IsVideoExtension(filepath.Ext(entry.Name())) {
			return nil
		}

		relPath, err := i.relPathFor(absPath)
		if err != nil {
			slog.Warn("metadata index skipped file",
				slog.String("action", "metadata.index"),
				slog.String("library_id", library.ID),
				slog.String("path", absPath),
				slog.String("error", err.Error()),
			)

			return nil
		}

		indexed[relPath] = struct{}{}

		info, err := entry.Info()
		if err != nil {
			return nil //nolint:nilerr // skip unreadable files during indexing
		}

		needsProbe, err := i.store.NeedsProbe(ctx, library.ID, relPath, info.ModTime(), info.Size())
		if err != nil {
			slog.Warn("metadata index probe check failed",
				slog.String("action", "metadata.index"),
				slog.String("library_id", library.ID),
				slog.String("path", relPath),
				slog.String("error", err.Error()),
			)

			return nil
		}

		if !needsProbe {
			return nil
		}

		start := time.Now()
		fields, err := i.probeFile(ctx, absPath)
		status := "success"
		if err != nil {
			status = "error"
			observability.RecordMetadataProbe(status, time.Since(start))
			slog.Warn("metadata probe failed",
				slog.String("action", "metadata.probe"),
				slog.String("library_id", library.ID),
				slog.String("path", relPath),
				slog.String("error", err.Error()),
			)

			return nil
		}

		observability.RecordMetadataProbe(status, time.Since(start))

		var existingOriginal VideoFields
		existing, getErr := i.store.Get(ctx, library.ID, relPath)
		switch {
		case getErr == nil:
			existingOriginal = existing.Original
		case errors.Is(getErr, ErrNotFound):
			// first index
		default:
			slog.Warn("metadata index load existing failed",
				slog.String("action", "metadata.index"),
				slog.String("library_id", library.ID),
				slog.String("path", relPath),
				slog.String("error", getErr.Error()),
			)
		}

		fields = attachIdentityHashes(ctx, absPath, fields, existingOriginal)

		err = i.store.UpsertOriginal(
			ctx,
			library.ID,
			relPath,
			fields,
			info.ModTime(),
			info.Size(),
			time.Now().UTC(),
		)
		if err != nil {
			slog.Warn("metadata index upsert failed",
				slog.String("action", "metadata.index"),
				slog.String("library_id", library.ID),
				slog.String("path", relPath),
				slog.String("error", err.Error()),
			)

			return nil
		}

		filesIndexed++
		observability.RecordMetadataIndexFile(library.Slug)

		return nil
	})
	if walkErr != nil {
		slog.Warn("metadata index walk failed",
			slog.String("action", "metadata.index"),
			slog.String("library_id", library.ID),
			slog.String("library_slug", library.Slug),
			slog.String("root", root),
			slog.String("error", walkErr.Error()),
		)
	}

	return filesIndexed
}

func (i *Indexer) probeFile(ctx context.Context, absPath string) (VideoFields, error) {
	if i.probe != nil {
		return i.probe(ctx, absPath)
	}

	return ProbePathContext(ctx, absPath)
}

// attachIdentityHashes merges scan-time content hashes onto probed fields.
// Existing MovieHash / Ed2kHash values are never recomputed or overwritten.
func attachIdentityHashes(
	ctx context.Context,
	absPath string,
	fields VideoFields,
	existingOriginal VideoFields,
) VideoFields {
	out := fields
	attachMovieHash(absPath, &out, existingOriginal)
	attachEd2kHash(ctx, absPath, &out, existingOriginal)

	return out
}

func attachMovieHash(absPath string, out *VideoFields, existing VideoFields) {
	if stringPtrSet(existing.MovieHash) {
		out.MovieHash = existing.MovieHash
		if existing.HashFileSize != nil {
			out.HashFileSize = existing.HashFileSize
		}

		return
	}

	hash, size, err := mediahash.MovieHash(absPath)
	switch {
	case err == nil:
		out.MovieHash = &hash
		setHashFileSizeIfNil(out, size)
	case errors.Is(err, mediahash.ErrFileTooSmall):
		// leave unset — file under OpenSubtitles minimum size
	default:
		slog.Debug("moviehash failed",
			slog.String("action", "metadata.hash"),
			slog.String("path", absPath),
			slog.String("error", err.Error()),
		)
	}
}

func attachEd2kHash(ctx context.Context, absPath string, out *VideoFields, existing VideoFields) {
	if stringPtrSet(existing.Ed2kHash) {
		out.Ed2kHash = existing.Ed2kHash
		if out.HashFileSize == nil && existing.HashFileSize != nil {
			out.HashFileSize = existing.HashFileSize
		}

		return
	}

	slog.Debug("ed2k hash start",
		slog.String("action", "metadata.hash"),
		slog.String("path", absPath),
	)
	hash, size, err := mediahash.Ed2kContext(ctx, absPath)
	if err != nil {
		slog.Debug("ed2k hash failed",
			slog.String("action", "metadata.hash"),
			slog.String("path", absPath),
			slog.String("error", err.Error()),
		)

		return
	}

	out.Ed2kHash = &hash
	setHashFileSizeIfNil(out, size)
	slog.Debug("ed2k hash done",
		slog.String("action", "metadata.hash"),
		slog.String("path", absPath),
	)
}

func setHashFileSizeIfNil(fields *VideoFields, size int64) {
	if fields.HashFileSize == nil {
		fields.HashFileSize = &size
	}
}

func stringPtrSet(value *string) bool {
	return value != nil && *value != ""
}

// IdentityHashesIncomplete reports whether scan-time hashes still need backfill.
// MovieHash is only required when the file is large enough for OpenSubtitles moviehash.
func IdentityHashesIncomplete(fields VideoFields, fileSize int64) bool {
	if !stringPtrSet(fields.Ed2kHash) {
		return true
	}
	if fileSize >= mediahash.MovieHashMinSize && !stringPtrSet(fields.MovieHash) {
		return true
	}

	return false
}

func (i *Indexer) deleteStalePaths(
	ctx context.Context,
	library access.Library,
	indexed map[string]struct{},
) {
	paths, err := i.store.ListIndexedPaths(ctx, library.ID)
	if err != nil {
		slog.Warn("metadata index list stale failed",
			slog.String("action", "metadata.index"),
			slog.String("library_id", library.ID),
			slog.String("error", err.Error()),
		)

		return
	}

	var frozen []string
	if i.frozen != nil {
		frozen, err = i.frozen.OriginalPrefixes(ctx)
		if err != nil {
			slog.Warn("metadata index list trash prefixes failed",
				slog.String("action", "metadata.index"),
				slog.String("error", err.Error()),
			)
		}
	}

	for _, relPath := range paths {
		if _, ok := indexed[relPath]; ok {
			continue
		}
		if frozenPath(relPath, frozen) {
			continue
		}

		err = i.store.DeletePath(ctx, library.ID, relPath)
		if err != nil {
			slog.Warn("metadata index delete stale failed",
				slog.String("action", "metadata.index"),
				slog.String("library_id", library.ID),
				slog.String("path", relPath),
				slog.String("error", err.Error()),
			)
		}
	}
}

func (i *Indexer) relPathFor(absPath string) (string, error) {
	mediaRoot := i.media.Root()
	rel, err := filepath.Rel(mediaRoot, absPath)
	if err != nil {
		return "", fmt.Errorf("rel path: %w", err)
	}

	return filepath.ToSlash(rel), nil
}

func frozenPath(relPath string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if access.PathWithinRoot(relPath, prefix) {
			return true
		}
	}

	return false
}

// LibraryCatalog lists registered libraries for metadata resolution.
type LibraryCatalog interface {
	ListLibraries(ctx context.Context) ([]access.Library, error)
}

// Service coordinates metadata reads and writes.
type Service struct {
	media     *mediafs.Service
	access    LibraryCatalog
	store     Repository
	indexer   *Indexer
	writeTags func(ctx context.Context, absPath string, tags map[string]string) error
	probe     func(ctx context.Context, absPath string) (VideoFields, error)
}

// Repository persists metadata rows for the domain service.
type Repository interface {
	Get(ctx context.Context, libraryID, relPath string) (Row, error)
	UpdateOverride(
		ctx context.Context,
		libraryID, relPath string,
		override StoredOverride,
		updatedAt time.Time,
		overriddenBy string,
	) error
	UpsertOriginal(
		ctx context.Context,
		libraryID, relPath string,
		original VideoFields,
		fileMtime time.Time,
		fileSize int64,
		probedAt time.Time,
	) error
	DeletePath(ctx context.Context, libraryID, relPath string) error
	Search(
		ctx context.Context,
		libraryIDs []string,
		tokens []string,
		rawQuery string,
		limit, offset int,
	) ([]SearchRow, int, error)
	ListActiveIndexedPaths(ctx context.Context, libraryID string) ([]string, error)
}

// NewService constructs a metadata service.
func NewService(
	media *mediafs.Service,
	accessService LibraryCatalog,
	store Repository,
	indexer *Indexer,
) *Service {
	return &Service{
		media:   media,
		access:  accessService,
		store:   store,
		indexer: indexer,
	}
}

// Indexer returns the background indexer.
func (s *Service) Indexer() *Indexer {
	if s == nil {
		return nil
	}

	return s.indexer
}

// ListIndexedPaths returns active (non-trash) indexed media paths for a library.
// Used by catalog request paths so discovery never walks the filesystem.
func (s *Service) ListIndexedPaths(ctx context.Context, libraryID string) ([]string, error) {
	if s == nil || s.store == nil || libraryID == "" {
		return nil, nil
	}

	paths, err := s.store.ListActiveIndexedPaths(ctx, libraryID)
	if err != nil {
		return nil, fmt.Errorf("list indexed catalog paths: %w", err)
	}

	return paths, nil
}

// DeleteForPath removes cached metadata for a media path.
func (s *Service) DeleteForPath(ctx context.Context, rawPath string) error {
	if s == nil || s.store == nil || s.access == nil {
		return nil
	}

	relPath := normalizeMediaPath(rawPath)
	if relPath == "" {
		return mediafs.ErrInvalidPath
	}

	library, _, lookupErr := s.libraryForPath(ctx, relPath)
	if lookupErr != nil {
		if errors.Is(lookupErr, ErrUnknownLibrary) {
			return nil
		}

		return fmt.Errorf("lookup library for delete: %w", lookupErr)
	}

	delErr := s.store.DeletePath(ctx, library.ID, relPath)
	if delErr != nil {
		return fmt.Errorf("delete metadata path: %w", delErr)
	}

	return nil
}

// Search returns indexed videos matching query tokens in readable libraries.
func (s *Service) Search( //nolint:cyclop // hydrate hits from rows + library map
	ctx context.Context,
	libraryIDs []string,
	query string,
	limit, offset int,
) ([]SearchHit, int, error) {
	if s == nil || s.store == nil || s.access == nil {
		return nil, 0, ErrServiceUnavailable
	}

	tokens := NormalizeSearchQuery(query)
	if len(libraryIDs) == 0 || len(tokens) == 0 {
		return []SearchHit{}, 0, nil
	}

	opts := mediafs.NormalizePageOpts(mediafs.PageOpts{Limit: limit, Offset: offset})
	rows, total, err := s.store.Search(
		ctx,
		libraryIDs,
		tokens,
		strings.Join(tokens, " "),
		opts.Limit,
		opts.Offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("search metadata: %w", err)
	}

	libraries, err := s.access.ListLibraries(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("list libraries: %w", err)
	}

	byID := make(map[string]access.Library, len(libraries))
	for _, library := range libraries {
		byID[library.ID] = library
	}

	hits := make([]SearchHit, 0, len(rows))
	for _, row := range rows {
		if access.IsHiddenRelPath(row.RelPath) {
			continue
		}

		library, ok := byID[row.LibraryID]
		if !ok {
			continue
		}

		stored, getErr := s.store.Get(ctx, row.LibraryID, row.RelPath)
		original := VideoFields{}
		override := StoredOverride{}
		if getErr == nil {
			original = stored.Original
			override = stored.Override
		} else if !errors.Is(getErr, ErrNotFound) {
			return nil, 0, fmt.Errorf("hydrate search hit: %w", getErr)
		}

		hit := SearchHit{
			Path:        row.RelPath,
			Title:       DisplayName(library.Type, row.RelPath, original, override),
			LibrarySlug: library.Slug,
			LibraryType: library.Type,
		}
		if hit.LibrarySlug == "" {
			hit.LibrarySlug = library.RelPath
		}
		if library.Type == access.LibraryTypeSeries {
			hit.ShowKey = ParseSeriesIdentity(row.RelPath).ShowKey
		}

		hits = append(hits, hit)
	}

	return hits, total, nil
}

// Get returns metadata for a media path.
func (s *Service) Get(ctx context.Context, rawPath string) (MetadataResponse, error) {
	if s == nil || s.access == nil || s.store == nil {
		return MetadataResponse{}, ErrServiceUnavailable
	}

	relPath := normalizeMediaPath(rawPath)
	if relPath == "" {
		return MetadataResponse{}, mediafs.ErrInvalidPath
	}

	if !s.isVideoMediaPath(relPath) {
		return MetadataResponse{}, ErrNotVideo
	}

	library, libraryType, err := s.libraryForPath(ctx, relPath)
	if err != nil {
		return MetadataResponse{}, err
	}

	row, err := s.store.Get(ctx, library.ID, relPath)
	original := VideoFields{}
	override := StoredOverride{}
	var probedAt *time.Time
	var overriddenBy *string

	if err == nil {
		original = row.Original
		override = row.Override
		probedAt = row.ProbedAt
		overriddenBy = row.OverriddenBy
	} else if !errors.Is(err, ErrNotFound) {
		return MetadataResponse{}, fmt.Errorf("load metadata: %w", err)
	}

	effective := EffectiveFields(libraryType, relPath, original, override)

	return MetadataResponse{
		Path:                   relPath,
		LibraryType:            libraryType,
		UsesMetadataForDisplay: UsesMetadataForDisplay(libraryType),
		Source: SourceFields{
			Original: original,
			Override: override.VideoFields,
		},
		Effective:    effective,
		DisplayName:  DisplayName(libraryType, relPath, original, override),
		OverriddenBy: overriddenBy,
		FilenameHint: FilenameHintForPath(relPath),
		ProbedAt:     probedAt,
	}, nil
}

// Patch updates metadata for a media path.
func (s *Service) Patch(
	ctx context.Context,
	rawPath string,
	request PatchRequest,
	userID string,
) (MetadataResponse, error) {
	if s == nil || s.access == nil || s.store == nil {
		return MetadataResponse{}, ErrServiceUnavailable
	}

	return s.patch(ctx, rawPath, request, FormatUserOverriddenBy(userID))
}

func (s *Service) patch(
	ctx context.Context,
	rawPath string,
	request PatchRequest,
	overriddenBy string,
) (MetadataResponse, error) {
	switch request.Target {
	case PatchTargetOverride:
		return s.patchOverride(ctx, rawPath, request.Fields, overriddenBy)
	case PatchTargetFile:
		return s.patchFile(ctx, rawPath, request.Fields, overriddenBy)
	case "":
		return MetadataResponse{}, ErrInvalidTarget
	default:
		return MetadataResponse{}, ErrInvalidTarget
	}
}

func (s *Service) patchOverride(
	ctx context.Context,
	rawPath string,
	fields map[string]*jsonValue,
	overriddenBy string,
) (MetadataResponse, error) {
	relPath := normalizeMediaPath(rawPath)
	if relPath == "" {
		return MetadataResponse{}, mediafs.ErrInvalidPath
	}

	if !s.isVideoMediaPath(relPath) {
		return MetadataResponse{}, ErrNotVideo
	}

	library, _, err := s.libraryForPath(ctx, relPath)
	if err != nil {
		return MetadataResponse{}, err
	}

	current := StoredOverride{}
	row, err := s.store.Get(ctx, library.ID, relPath)
	if err == nil {
		current = row.Override
	} else if !errors.Is(err, ErrNotFound) {
		return MetadataResponse{}, fmt.Errorf("load metadata: %w", err)
	}

	updated, err := ApplyPatchFields(current, fields)
	if err != nil {
		return MetadataResponse{}, err
	}

	err = s.store.UpdateOverride(
		ctx,
		library.ID,
		relPath,
		updated,
		time.Now().UTC(),
		overriddenBy,
	)
	if err != nil {
		return MetadataResponse{}, fmt.Errorf("update override metadata: %w", err)
	}

	return s.Get(ctx, relPath)
}

func (s *Service) patchFile( //nolint:cyclop // file-tag patch handles many field branches
	ctx context.Context,
	rawPath string,
	fields map[string]*jsonValue,
	overriddenBy string,
) (MetadataResponse, error) {
	if s.media == nil {
		return MetadataResponse{}, ErrFileTargetUnsupported
	}

	relPath := normalizeMediaPath(rawPath)
	if relPath == "" {
		return MetadataResponse{}, mediafs.ErrInvalidPath
	}

	if !s.isVideoMediaPath(relPath) {
		return MetadataResponse{}, ErrNotVideo
	}

	library, _, err := s.libraryForPath(ctx, relPath)
	if err != nil {
		return MetadataResponse{}, err
	}

	current := VideoFields{}
	override := StoredOverride{}
	row, err := s.store.Get(ctx, library.ID, relPath)
	if err == nil {
		current = row.Original
		override = row.Override
	} else if !errors.Is(err, ErrNotFound) {
		return MetadataResponse{}, fmt.Errorf("load metadata: %w", err)
	}

	updated, err := ApplyPatchFields(StoredOverride{VideoFields: current}, fields)
	if err != nil {
		return MetadataResponse{}, err
	}

	absPath, err := s.media.FilePath(relPath)
	if err != nil {
		return MetadataResponse{}, fmt.Errorf("resolve media file: %w", err)
	}

	err = s.writeFileTags(ctx, absPath, fileTagUpdates(fields, updated.VideoFields))
	if err != nil {
		return MetadataResponse{}, err
	}

	err = s.persistAfterFileWrite(ctx, library.ID, relPath, absPath, override, fields, overriddenBy)
	if err != nil {
		return MetadataResponse{}, err
	}

	return s.Get(ctx, relPath)
}

func (s *Service) persistAfterFileWrite(
	ctx context.Context,
	libraryID, relPath, absPath string,
	override StoredOverride,
	fields map[string]*jsonValue,
	overriddenBy string,
) error {
	start := time.Now()
	probed, err := s.probeFile(ctx, absPath)
	if err != nil {
		observability.RecordMetadataProbe("error", time.Since(start))

		return fmt.Errorf("probe rewritten file: %w", err)
	}

	observability.RecordMetadataProbe("success", time.Since(start))

	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("stat rewritten file: %w", err)
	}

	err = s.store.UpsertOriginal(
		ctx,
		libraryID,
		relPath,
		probed,
		info.ModTime(),
		info.Size(),
		time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("update original metadata: %w", err)
	}

	cleared := ClearOverrideKeys(override, fields)
	err = s.store.UpdateOverride(
		ctx,
		libraryID,
		relPath,
		cleared,
		time.Now().UTC(),
		overriddenBy,
	)
	if err != nil {
		return fmt.Errorf("clear override after file write: %w", err)
	}

	return nil
}

func (s *Service) writeFileTags(
	ctx context.Context,
	absPath string,
	tags map[string]string,
) error {
	if s.writeTags != nil {
		return s.writeTags(ctx, absPath, tags)
	}

	return WriteFileTags(ctx, absPath, tags)
}

func (s *Service) probeFile(ctx context.Context, absPath string) (VideoFields, error) {
	if s.probe != nil {
		return s.probe(ctx, absPath)
	}

	return ProbePathContext(ctx, absPath)
}

func (s *Service) libraryForPath(
	ctx context.Context,
	relPath string,
) (access.Library, access.LibraryType, error) {
	libraries, err := s.access.ListLibraries(ctx)
	if err != nil {
		return access.Library{}, "", fmt.Errorf("list libraries: %w", err)
	}

	library, ok := access.MatchLibrary(libraries, relPath)
	if !ok {
		return access.Library{}, "", fmt.Errorf("%w: %s", ErrUnknownLibrary, relPath)
	}

	return library, library.Type, nil
}

func normalizeMediaPath(rawPath string) string {
	cleaned := strings.TrimSpace(rawPath)
	cleaned = strings.TrimPrefix(cleaned, "/")
	cleaned = filepath.ToSlash(cleaned)

	return cleaned
}

func (s *Service) isVideoMediaPath(relPath string) bool {
	if IsVideoExtension(filepath.Ext(relPath)) {
		return true
	}
	if s == nil || s.media == nil {
		return false
	}

	absPath, err := s.media.FilePath(relPath)
	if err != nil {
		return false
	}

	mimeType := mediafs.DetectMimeType(absPath, filepath.Ext(relPath), false)

	return strings.HasPrefix(mimeType, "video/")
}
