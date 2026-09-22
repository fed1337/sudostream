package provider

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Service validates and persists per-library provider settings.
type Service struct {
	store    Store
	registry *Registry
}

// NewService constructs a provider settings service.
func NewService(store Store, registry *Registry) *Service {
	if registry == nil {
		registry = NewRegistry()
	}

	return &Service{store: store, registry: registry}
}

// Registry returns the adapter registry backing this service.
func (s *Service) Registry() *Registry {
	return s.registry
}

// GetSettings returns library's provider settings, or the all-empty default when never configured.
func (s *Service) GetSettings(ctx context.Context, libraryID string) (Settings, error) {
	settings, err := s.store.GetSettings(ctx, libraryID)
	if errors.Is(err, ErrNotFound) {
		return DefaultSettings(libraryID), nil
	}
	if err != nil {
		return Settings{}, fmt.Errorf("get provider settings: %w", err)
	}

	return settings, nil
}

// PatchSettings validates and replaces libraryID's provider settings.
func (s *Service) PatchSettings(
	ctx context.Context,
	libraryID string,
	input Settings,
) (Settings, error) {
	input.LibraryID = libraryID

	err := s.validate(&input)
	if err != nil {
		return Settings{}, err
	}

	saved, err := s.store.UpsertSettings(ctx, input)
	if err != nil {
		return Settings{}, fmt.Errorf("save provider settings: %w", err)
	}

	return saved, nil
}

//nolint:cyclop // straight-line validation of independent fields
func (s *Service) validate(input *Settings) error {
	if input.MetadataProvider != nil && !s.registry.SupportsMetadata(*input.MetadataProvider) {
		return ErrInvalidMetadataProvider
	}
	if input.PosterProvider != nil && !s.registry.SupportsPoster(*input.PosterProvider) {
		return ErrInvalidPosterProvider
	}
	if input.SubtitleProvider != nil && !s.registry.SupportsSubtitle(*input.SubtitleProvider) {
		return ErrInvalidSubtitleProvider
	}

	switch input.MetadataApplyMode {
	case ApplyModeFillMissing, ApplyModeFullRewrite:
	default:
		return ErrInvalidApplyMode
	}

	switch input.MetadataWriteTarget {
	case WriteTargetDB, WriteTargetFile:
	default:
		return ErrInvalidWriteTarget
	}

	langs, err := normalizeSubtitleLanguages(input.SubtitleLanguages)
	if err != nil {
		return err
	}

	if input.SubtitleProvider == nil {
		langs = []string{}
	} else if len(langs) == 0 {
		return ErrSubtitleLanguagesRequired
	}
	input.SubtitleLanguages = langs

	return nil
}

func normalizeSubtitleLanguages(codes []string) ([]string, error) {
	seen := make(map[string]struct{}, len(codes))
	out := make([]string, 0, len(codes))

	for _, raw := range codes {
		code := strings.ToLower(strings.TrimSpace(raw))
		if code == "" {
			continue
		}
		if !IsSubtitleLanguage(code) {
			return nil, ErrInvalidSubtitleLanguage
		}
		if _, dup := seen[code]; dup {
			continue
		}
		seen[code] = struct{}{}
		out = append(out, code)
	}

	slices.Sort(out)

	return out, nil
}
