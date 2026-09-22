package transcode

import (
	"errors"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// ErrInvalidVariant is returned when a play URL does not address a known rendition.
var ErrInvalidVariant = errors.New("invalid variant")

// minVariantResourceLen is the `<variantDir>/<file>` segment count of a rendition resource.
const minVariantResourceLen = 2

// VariantKind distinguishes the rendition families published in the master playlist.
type VariantKind string

const (
	// VariantVideo is a video-only ladder rung (`v1080`).
	VariantVideo VariantKind = "video"
	// VariantAudio is an alternate audio rendition (`a0`).
	VariantAudio VariantKind = "audio"
)

// Variant identifies one HLS rendition within a transcode cache directory.
type Variant struct {
	Kind VariantKind
	// Value is the ladder height for video and the source track ordinal for audio.
	Value int
}

// Dir returns the cache subdirectory holding this rendition's segments.
func (v Variant) Dir() string {
	if v.Kind == VariantAudio {
		return AudioDir(v.Value)
	}

	return VariantDir(v.Value)
}

// VideoVariant builds a ladder rung identifier.
func VideoVariant(height int) Variant {
	return Variant{Kind: VariantVideo, Value: height}
}

// AudioVariant builds an alternate audio rendition identifier.
func AudioVariant(trackIndex int) Variant {
	return Variant{Kind: VariantAudio, Value: trackIndex}
}

// ParseVariantDir maps `v1080` or `a2` back to a rendition identifier.
func ParseVariantDir(dir string) (Variant, bool) {
	if len(dir) < 2 { //nolint:mnd // prefix letter plus at least one digit
		return Variant{}, false
	}

	value, err := strconv.Atoi(dir[1:])
	if err != nil || value < 0 {
		return Variant{}, false
	}

	switch dir[0] {
	case 'v':
		if value <= 0 {
			return Variant{}, false
		}

		return VideoVariant(value), true
	case 'a':
		return AudioVariant(value), true
	default:
		return Variant{}, false
	}
}

// SplitSegmentResource maps `v1080/seg_00042.ts` to its rendition and segment index.
func SplitSegmentResource(resource string) (Variant, int, bool) {
	parts := strings.Split(filepath.ToSlash(strings.TrimPrefix(resource, "/")), "/")
	if len(parts) != minVariantResourceLen {
		return Variant{}, 0, false
	}

	variant, ok := ParseVariantDir(parts[0])
	if !ok {
		return Variant{}, 0, false
	}

	index, ok := SegmentIndexFromName(parts[1])
	if !ok {
		return Variant{}, 0, false
	}

	return variant, index, true
}

// VariantHeightFromResource parses v{height}/... style resources.
func VariantHeightFromResource(resource string) (int, bool) {
	clean := filepath.ToSlash(strings.TrimPrefix(resource, "/"))

	parts := strings.Split(clean, "/")
	if len(parts) < minVariantResourceLen {
		return 0, false
	}

	variant, ok := ParseVariantDir(parts[0])
	if !ok || variant.Kind != VariantVideo {
		return 0, false
	}

	return variant.Value, true
}

// validVariant reports whether a rendition exists for the cached source.
func validVariant(meta SourceMeta, variant Variant) bool {
	if variant.Kind == VariantAudio {
		return variant.Value >= 0 && variant.Value < len(meta.AudioStreams)
	}

	return slices.Contains(LadderHeights(meta.Height), variant.Value)
}
