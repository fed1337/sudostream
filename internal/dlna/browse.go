package dlna

import (
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sudoStream/internal/access"
	"sudoStream/internal/auth"
	"sudoStream/internal/catalog"
	"sudoStream/internal/mediafs"
	"sudoStream/internal/observability"
	"sudoStream/internal/playback"
	"sudoStream/internal/transcode"
	"time"
)

const (
	classStorageFolder = "object.container.storageFolder"
	objectIDLibDepth   = 2

	idRoot = "0"

	partRoot    = "0"
	partLib     = "lib"
	partCatalog = "cat"
	partFiles   = "files"
	partMovie   = "movie"
	partShow    = "show"
	partSeason  = "season"
	partEpisode = "ep"
	partPath    = "path"
)

// Browser builds ContentDirectory DIDL from catalog + files trees.
type Browser struct {
	Access  *access.Service
	Media   *mediafs.Service
	Catalog *catalog.Service
	Probe   func(ctx context.Context, absPath string) (transcode.SourceInfo, error)
	BaseURL string // http://lan:8200
	SignKey []byte
	Now     func() time.Time
}

type didlLite struct {
	XMLName    xml.Name        `xml:"DIDL-Lite"`
	Xmlns      string          `xml:"xmlns,attr"`
	DC         string          `xml:"xmlns:dc,attr"`
	UPNP       string          `xml:"xmlns:upnp,attr"`
	Items      []didlItem      `xml:"item"`
	Containers []didlContainer `xml:"container"`
}

type didlContainer struct {
	ID         string `xml:"id,attr"`
	ParentID   string `xml:"parentID,attr"`
	Restricted string `xml:"restricted,attr"`
	Title      string `xml:"dc:title"`
	Class      string `xml:"upnp:class"`
}

type didlItem struct {
	ID         string   `xml:"id,attr"`
	ParentID   string   `xml:"parentID,attr"`
	Restricted string   `xml:"restricted,attr"`
	Title      string   `xml:"dc:title"`
	Class      string   `xml:"upnp:class"`
	Res        *didlRes `xml:"res,omitempty"`
}

type didlRes struct {
	ProtocolInfo string `xml:"protocolInfo,attr"`
	Duration     string `xml:"duration,attr,omitempty"`
	Size         string `xml:"size,attr,omitempty"`
	URL          string `xml:",chardata"`
}

type browseEntry struct {
	ID           string
	ParentID     string
	Title        string
	Class        string
	IsContainer  bool
	ResURL       string
	ProtocolInfo string
	Duration     string
	Size         string
}

// BrowseDirectChildren returns DIDL-Lite XML and totalMatches for ObjectID.
func (b *Browser) BrowseDirectChildren( //nolint:cyclop // DIDL page assembly
	ctx context.Context,
	user auth.PublicUser,
	objectID string,
	startingIndex, requestedCount int,
) (string, int, error) {
	if requestedCount <= 0 || requestedCount > browsePageSize {
		requestedCount = browsePageSize
	}
	if startingIndex < 0 {
		startingIndex = 0
	}

	parts, err := DecodeObjectID(objectID)
	if err != nil {
		return "", 0, err
	}

	entries, err := b.children(ctx, user, parts)
	if err != nil {
		return "", 0, err
	}

	total := len(entries)
	if startingIndex >= total {
		return emptyDIDL(), total, nil
	}
	end := min(startingIndex+requestedCount, total)

	page := entries[startingIndex:end]
	doc := didlLite{
		Xmlns: "urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/",
		DC:    "http://purl.org/dc/elements/1.1/",
		UPNP:  "urn:schemas-upnp-org:metadata-1-0/upnp/",
	}
	for _, entry := range page {
		if entry.IsContainer {
			doc.Containers = append(doc.Containers, didlContainer{
				ID:         entry.ID,
				ParentID:   entry.ParentID,
				Restricted: "1",
				Title:      entry.Title,
				Class:      entry.Class,
			})

			continue
		}
		item := didlItem{
			ID:         entry.ID,
			ParentID:   entry.ParentID,
			Restricted: "1",
			Title:      entry.Title,
			Class:      "object.item.videoItem",
		}
		if entry.ResURL != "" {
			item.Res = &didlRes{
				ProtocolInfo: entry.ProtocolInfo,
				Duration:     entry.Duration,
				Size:         entry.Size,
				URL:          entry.ResURL,
			}
		}
		doc.Items = append(doc.Items, item)
	}

	raw, err := xml.Marshal(doc)
	if err != nil {
		return "", 0, fmt.Errorf("marshal didl: %w", err)
	}

	return string(raw), total, nil
}

func (b *Browser) children(
	ctx context.Context,
	user auth.PublicUser,
	parts []string,
) ([]browseEntry, error) {
	if len(parts) == 0 || parts[0] == partRoot {
		return b.rootLibraries(ctx, user)
	}

	if parts[0] != partLib || len(parts) < objectIDLibDepth {
		return nil, ErrUnknownObjectID
	}

	libraryID := parts[1]
	if len(parts) == objectIDLibDepth {
		return b.libraryBranches(libraryID), nil
	}

	switch parts[2] {
	case partCatalog:
		return b.catalogChildren(ctx, user, libraryID, parts[3:])
	case partFiles:
		return b.filesChildren(ctx, user, libraryID, parts[3:])
	default:
		return nil, ErrUnknownObjectID
	}
}

func (b *Browser) rootLibraries(ctx context.Context, user auth.PublicUser) ([]browseEntry, error) {
	libraries, err := b.Access.ListReadableLibraries(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("list readable libraries: %w", err)
	}

	out := make([]browseEntry, 0, len(libraries))
	for _, library := range libraries {
		if library.Type == access.LibraryTypeMusic || library.Type == access.LibraryTypePhotos {
			continue
		}
		out = append(out, browseEntry{
			ID:          EncodeObjectID(partLib, library.ID),
			ParentID:    idRoot,
			Title:       library.Name,
			Class:       classStorageFolder,
			IsContainer: true,
		})
	}

	return out, nil
}

func (b *Browser) libraryBranches(libraryID string) []browseEntry {
	parent := EncodeObjectID(partLib, libraryID)

	return []browseEntry{
		{
			ID:          EncodeObjectID(partLib, libraryID, partCatalog),
			ParentID:    parent,
			Title:       "Catalog",
			Class:       classStorageFolder,
			IsContainer: true,
		},
		{
			ID:          EncodeObjectID(partLib, libraryID, partFiles),
			ParentID:    parent,
			Title:       "Files",
			Class:       classStorageFolder,
			IsContainer: true,
		},
	}
}

func (b *Browser) findLibrary(
	ctx context.Context,
	user auth.PublicUser,
	libraryID string,
) (access.Library, error) {
	libraries, err := b.Access.ListReadableLibraries(ctx, user)
	if err != nil {
		return access.Library{}, fmt.Errorf("list readable libraries: %w", err)
	}
	for _, candidate := range libraries {
		if candidate.ID == libraryID {
			return candidate, nil
		}
	}

	return access.Library{}, access.ErrLibraryNotFound
}

func (b *Browser) catalogChildren( //nolint:cyclop // film vs series branches
	ctx context.Context,
	user auth.PublicUser,
	libraryID string,
	rest []string,
) ([]browseEntry, error) {
	if b.Catalog == nil {
		return nil, nil
	}

	library, err := b.findLibrary(ctx, user, libraryID)
	if err != nil {
		return nil, fmt.Errorf("find library: %w", err)
	}

	parentID := EncodeObjectID(partLib, libraryID, partCatalog)
	if len(rest) > 0 {
		parentID = EncodeObjectID(append([]string{partLib, libraryID, partCatalog}, rest...)...)
	}

	switch library.Type {
	case access.LibraryTypeFilm:
		if len(rest) > 0 {
			return nil, nil
		}
		page, catErr := b.Catalog.GetLibraryCatalog(ctx, user, library.Slug, mediafs.PageOpts{
			Limit: mediafs.MaxPageLimit,
		})
		if catErr != nil {
			return nil, fmt.Errorf("catalog films: %w", catErr)
		}
		out := make([]browseEntry, 0, len(page.Movies))
		for _, movie := range page.Movies {
			entry, itemErr := b.videoItem(
				ctx,
				user,
				EncodeObjectID(partLib, libraryID, partCatalog, partMovie, movie.Path),
				parentID,
				movie.Title,
				movie.Path,
			)
			if itemErr != nil {
				continue
			}
			out = append(out, entry)
		}

		return out, nil
	case access.LibraryTypeSeries:
		return b.seriesCatalogChildren(ctx, user, library, rest, parentID)
	case access.LibraryTypeMusic, access.LibraryTypePhotos, access.LibraryTypeOther:
		return nil, nil
	default:
		return nil, nil
	}
}

func (b *Browser) seriesCatalogChildren( //nolint:cyclop // show/season/episode tree
	ctx context.Context,
	user auth.PublicUser,
	library access.Library,
	rest []string,
	parentID string,
) ([]browseEntry, error) {
	switch {
	case len(rest) == 0:
		page, err := b.Catalog.GetLibraryCatalog(ctx, user, library.Slug, mediafs.PageOpts{
			Limit: mediafs.MaxPageLimit,
		})
		if err != nil {
			return nil, fmt.Errorf("catalog shows: %w", err)
		}
		out := make([]browseEntry, 0, len(page.Shows))
		for _, show := range page.Shows {
			out = append(out, browseEntry{
				ID: EncodeObjectID(
					partLib, library.ID, partCatalog, partShow, show.ShowKey,
				),
				ParentID:    parentID,
				Title:       show.Name,
				Class:       classStorageFolder,
				IsContainer: true,
			})
		}

		return out, nil
	case len(rest) == 2 && rest[0] == partShow:
		show, err := b.Catalog.GetShow(ctx, user, library.Slug, rest[1])
		if err != nil {
			return nil, fmt.Errorf("get show: %w", err)
		}
		out := make([]browseEntry, 0, len(show.Seasons))
		for _, season := range show.Seasons {
			title := "Season " + strconv.Itoa(season.Season)
			if season.Season == 0 {
				title = "Specials"
			}
			out = append(out, browseEntry{
				ID: EncodeObjectID(
					partLib, library.ID, partCatalog, partShow, rest[1],
					partSeason, strconv.Itoa(season.Season),
				),
				ParentID:    parentID,
				Title:       title,
				Class:       classStorageFolder,
				IsContainer: true,
			})
		}

		return out, nil
	case len(rest) == 4 && rest[0] == partShow && rest[2] == partSeason:
		seasonNum, err := strconv.Atoi(rest[3])
		if err != nil {
			return nil, fmt.Errorf("parse season: %w", err)
		}
		seasonPage, err := b.Catalog.ListSeasonEpisodes(
			ctx, user, library.Slug, rest[1], seasonNum, mediafs.PageOpts{},
		)
		if err != nil {
			return nil, fmt.Errorf("list episodes: %w", err)
		}
		out := make([]browseEntry, 0, len(seasonPage.Episodes))
		for _, episode := range seasonPage.Episodes {
			title := episode.Title
			if title == "" {
				title = path.Base(episode.Path)
			}
			entry, itemErr := b.videoItem(
				ctx,
				user,
				EncodeObjectID(
					partLib, library.ID, partCatalog, partShow, rest[1],
					partSeason, rest[3], partEpisode, episode.Path,
				),
				parentID,
				title,
				episode.Path,
			)
			if itemErr != nil {
				continue
			}
			out = append(out, entry)
		}

		return out, nil
	default:
		return nil, nil
	}
}

func (b *Browser) filesChildren( //nolint:cyclop,funlen // multi-root + ACL filter
	ctx context.Context,
	user auth.PublicUser,
	libraryID string,
	rest []string,
) ([]browseEntry, error) {
	library, err := b.findLibrary(ctx, user, libraryID)
	if err != nil {
		return nil, fmt.Errorf("find library: %w", err)
	}

	roots := library.RootPaths()
	parentID := EncodeObjectID(partLib, libraryID, partFiles)
	var browsePath string

	switch {
	case len(rest) >= 2 && rest[0] == partPath:
		browsePath = strings.Join(rest[1:], "/")
		parentID = EncodeObjectID(append([]string{partLib, libraryID, partFiles, partPath}, rest[1:]...)...)
	case len(roots) > 1:
		out := make([]browseEntry, 0, len(roots))
		for _, root := range roots {
			root = strings.Trim(root, "/")
			out = append(out, browseEntry{
				ID: EncodeObjectID(
					partLib, libraryID, partFiles, partPath, root,
				),
				ParentID:    parentID,
				Title:       path.Base(root),
				Class:       classStorageFolder,
				IsContainer: true,
			})
		}

		return out, nil
	case len(roots) == 1:
		browsePath = strings.Trim(roots[0], "/")
	default:
		return nil, nil
	}

	listing, err := b.Media.Browse(browsePath)
	if err != nil {
		return nil, fmt.Errorf("browse files: %w", err)
	}

	out := make([]browseEntry, 0, len(listing.Folder.Children))
	for _, child := range listing.Folder.Children {
		allowed, readErr := b.Access.CanRead(ctx, user, child.Path)
		if readErr != nil || !allowed {
			continue
		}
		childPath := strings.TrimPrefix(child.Path, "/")
		objectID := EncodeObjectID(
			append([]string{partLib, libraryID, partFiles, partPath}, strings.Split(childPath, "/")...)...,
		)
		if child.IsDir {
			out = append(out, browseEntry{
				ID:          objectID,
				ParentID:    parentID,
				Title:       child.Name,
				Class:       classStorageFolder,
				IsContainer: true,
			})

			continue
		}
		if !mediafs.IsVideoExtension(path.Ext(child.Name)) {
			continue
		}
		entry, itemErr := b.videoItem(ctx, user, objectID, parentID, child.Name, childPath)
		if itemErr != nil {
			continue
		}
		if child.Size > 0 {
			entry.Size = strconv.FormatInt(child.Size, 10)
		}
		out = append(out, entry)
	}

	return out, nil
}

func (b *Browser) videoItem(
	ctx context.Context,
	user auth.PublicUser,
	objectID, parentID, title, mediaPath string,
) (browseEntry, error) {
	mediaPath = strings.TrimPrefix(mediaPath, "/")
	allowed, err := b.Access.CanRead(ctx, user, mediaPath)
	if err != nil || !allowed {
		return browseEntry{}, ErrReadDenied
	}

	mode, duration, size := b.streamMeta(ctx, mediaPath)

	now := time.Now().UTC()
	if b.Now != nil {
		now = b.Now()
	}
	query := SignStream(b.SignKey, user.ID, mediaPath, mode, now)
	escaped := pathEscapeKeepSlash(mediaPath)
	resURL := strings.TrimRight(b.BaseURL, "/") + "/dlna/stream/" + escaped + "?" + query

	mime := "video/mp4"
	if mode != StreamDirect {
		mime = "video/mpeg"
	}
	protocol := "http-get:*:" + mime + ":DLNA.ORG_OP=01;DLNA.ORG_CI=0;DLNA.ORG_FLAGS=01700000000000000000000000000000"

	return browseEntry{
		ID:           objectID,
		ParentID:     parentID,
		Title:        title,
		Class:        "object.item.videoItem",
		IsContainer:  false,
		ResURL:       resURL,
		ProtocolInfo: protocol,
		Duration:     duration,
		Size:         size,
	}, nil
}

func (b *Browser) streamMeta(ctx context.Context, mediaPath string) (StreamMode, string, string) {
	mode := StreamDirect
	if b.Probe == nil || b.Media == nil {
		return mode, "", ""
	}

	var duration, size string
	abs, pathErr := b.Media.FilePath(mediaPath)
	if pathErr == nil {
		info, probeErr := b.Probe(ctx, abs)
		if probeErr == nil {
			source := playback.SourceCaps{
				Container:  playback.ContainerFromPath(mediaPath),
				VideoCodec: info.VideoCodec,
				AudioCodec: info.AudioCodec,
				Bitrate:    info.Bitrate,
				Height:     info.Height,
				RemuxOK:    transcode.RemuxEligible(info),
			}
			decision := playback.Decide(TVDeviceProfile(), source, 0)
			observability.RecordPlaybackDecision(string(decision.Method), "dlna")
			mode = StreamModeForDecision(decision.Method)
			if info.DurationSeconds > 0 {
				duration = formatUPNPDuration(info.DurationSeconds)
			}
		}
	}

	file, fileInfo, openErr := b.Media.OpenFile(mediaPath)
	if openErr == nil {
		size = strconv.FormatInt(fileInfo.Size(), 10)
		_ = file.Close()
	}

	return mode, duration, size
}

func pathEscapeKeepSlash(rawPath string) string {
	parts := strings.Split(rawPath, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}

	return strings.Join(parts, "/")
}

func formatUPNPDuration(seconds float64) string {
	const (
		roundBias   = 0.5
		secsPerHour = 3600
		secsPerMin  = 60
	)
	total := int(seconds + roundBias)
	hours := total / secsPerHour
	mins := (total % secsPerHour) / secsPerMin
	secs := total % secsPerMin

	return fmt.Sprintf("%d:%02d:%02d.000", hours, mins, secs)
}

func emptyDIDL() string {
	return `<DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" ` +
		`xmlns:dc="http://purl.org/dc/elements/1.1/" ` +
		`xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/"></DIDL-Lite>`
}

// EscapeXML escapes text for embedding DIDL inside a SOAP body.
func EscapeXML(s string) string {
	return html.EscapeString(s)
}
