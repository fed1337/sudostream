package access

import "errors"

// LibraryType classifies a library for UI routing.
type LibraryType string

const (
	// LibraryTypeFilm is a film library.
	LibraryTypeFilm LibraryType = "film"
	// LibraryTypeSeries is a TV series library.
	LibraryTypeSeries LibraryType = "series"
	// LibraryTypeMusic is a music library.
	LibraryTypeMusic LibraryType = "music"
	// LibraryTypePhotos is a photo library.
	LibraryTypePhotos LibraryType = "photos"
	// LibraryTypeOther is a generic folder explorer library.
	LibraryTypeOther LibraryType = "other"
)

var (
	// ErrInvalidLibraryType is returned when a library type is not recognized.
	ErrInvalidLibraryType = errors.New("invalid library type")
	// ErrLibraryNotFound is returned when a library ID does not exist.
	ErrLibraryNotFound = errors.New("library not found")
	// ErrInvalidLibraryRoot is returned when a root path is empty or reserved.
	ErrInvalidLibraryRoot = errors.New("invalid library root")
	// ErrLibraryRootConflict is returned when a folder already belongs to a library.
	ErrLibraryRootConflict = errors.New("folder already belongs to a library")
	// ErrLibraryRootNotFound is returned when removing a root that is not registered.
	ErrLibraryRootNotFound = errors.New("library root not found")
	// ErrLibrarySlugConflict is returned when the requested slug is already used.
	ErrLibrarySlugConflict = errors.New("library slug already exists")
)

// ParseLibraryType validates and returns a library type.
func ParseLibraryType(raw string) (LibraryType, error) {
	switch LibraryType(raw) {
	case LibraryTypeFilm, LibraryTypeSeries, LibraryTypeMusic, LibraryTypePhotos, LibraryTypeOther:
		return LibraryType(raw), nil
	default:
		return "", ErrInvalidLibraryType
	}
}
