package anidb

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sudoStream/internal/provider"
	"sync"
	"time"
)

const (
	titlesCacheFile   = "anime-titles.xml"
	titlesCachePerm   = 0o600
	titlesDirPerm     = 0o750
	maxTitlesDumpSize = 64 << 20 // titles dump is larger than typical API responses
)

type titlesIndex struct {
	mu      sync.Mutex
	entries []titleEntry
	loaded  time.Time
}

type titleEntry struct {
	aid    string
	titles []string // normalized
}

func (p *Provider) loadTitles(ctx context.Context) (*titlesIndex, error) {
	if p.titles == nil {
		p.titles = &titlesIndex{}
	}
	p.titles.mu.Lock()
	defer p.titles.mu.Unlock()

	if p.titles.fresh() {
		return p.titles, nil
	}
	cachePath := filepath.Join(p.cacheDir, titlesCacheFile)
	if p.titles.loadFromDisk(cachePath) {
		return p.titles, nil
	}

	data, err := p.fetchTitles(ctx)
	if err != nil {
		return nil, err
	}
	entries, err := parseTitlesXML(data)
	if err != nil {
		return nil, err
	}
	err = p.writeTitlesCache(cachePath, data)
	if err != nil {
		return nil, err
	}
	p.titles.entries = entries
	p.titles.loaded = time.Now()

	return p.titles, nil
}

func (idx *titlesIndex) fresh() bool {
	return len(idx.entries) > 0 && time.Since(idx.loaded) < titlesCacheTTL
}

func (idx *titlesIndex) loadFromDisk(cachePath string) bool {
	info, err := os.Stat(cachePath)
	if err != nil || time.Since(info.ModTime()) >= titlesCacheTTL {
		return false
	}
	data, readErr := os.ReadFile(cachePath)
	if readErr != nil {
		return false
	}
	entries, parseErr := parseTitlesXML(data)
	if parseErr != nil {
		return false
	}
	idx.entries = entries
	idx.loaded = time.Now()

	return true
}

func (p *Provider) fetchTitles(ctx context.Context) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, p.titlesURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build anidb titles request: %w", err)
	}
	request.Header.Set("Accept-Encoding", "gzip")
	response, err := p.http.DoWithRetry(request)
	if err != nil {
		return nil, fmt.Errorf("fetch anidb titles: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusForbidden ||
		response.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf(
			"%w: %w: titles status %d",
			provider.ErrProviderUnavailable,
			ErrRequestFailed,
			response.StatusCode,
		)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: titles status %d", ErrRequestFailed, response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxTitlesDumpSize+1))
	if err != nil {
		return nil, fmt.Errorf("read anidb titles: %w", err)
	}
	if len(raw) > maxTitlesDumpSize {
		return nil, fmt.Errorf("%w: titles dump too large", ErrRequestFailed)
	}

	return decodeTitlesBody(raw, response.Header.Get("Content-Encoding"))
}

func decodeTitlesBody(raw []byte, contentEncoding string) ([]byte, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("%w: empty titles dump", ErrRequestFailed)
	}
	// Plain XML (tests via WithTitlesURL) — body starts with '<' or XML declaration.
	if trimmed[0] == '<' {
		return trimmed, nil
	}
	encoding := strings.ToLower(strings.TrimSpace(contentEncoding))
	needsGunzip := encoding == "gzip" ||
		(len(trimmed) >= 2 && trimmed[0] == 0x1f && trimmed[1] == 0x8b)
	if !needsGunzip {
		return trimmed, nil
	}
	reader, err := gzip.NewReader(bytes.NewReader(trimmed))
	if err != nil {
		return nil, fmt.Errorf("gunzip anidb titles: %w", err)
	}
	defer func() { _ = reader.Close() }()
	data, err := io.ReadAll(io.LimitReader(reader, maxTitlesDumpSize+1))
	if err != nil {
		return nil, fmt.Errorf("read gunzipped anidb titles: %w", err)
	}
	if len(data) > maxTitlesDumpSize {
		return nil, fmt.Errorf("%w: titles dump too large", ErrRequestFailed)
	}

	return data, nil
}

func (p *Provider) writeTitlesCache(cachePath string, data []byte) error {
	err := os.MkdirAll(filepath.Dir(cachePath), titlesDirPerm)
	if err != nil {
		return fmt.Errorf("create anidb titles cache dir: %w", err)
	}
	temp, err := os.CreateTemp(filepath.Dir(cachePath), ".titles-*")
	if err != nil {
		return fmt.Errorf("create anidb titles temp: %w", err)
	}
	tempPath := temp.Name()
	defer func() { _ = os.Remove(tempPath) }()

	_, err = temp.Write(data)
	closeErr := temp.Close()
	if err != nil {
		return fmt.Errorf("write anidb titles temp: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("close anidb titles temp: %w", closeErr)
	}
	err = os.Chmod(tempPath, titlesCachePerm)
	if err != nil {
		return fmt.Errorf("chmod anidb titles temp: %w", err)
	}
	err = os.Rename(tempPath, cachePath)
	if err != nil {
		return fmt.Errorf("publish anidb titles cache: %w", err)
	}

	return nil
}

func parseTitlesXML(data []byte) ([]titleEntry, error) {
	var dump titlesDumpDTO
	err := xml.Unmarshal(data, &dump)
	if err != nil {
		return nil, fmt.Errorf("decode anidb titles dump: %w", err)
	}
	entries := make([]titleEntry, 0, len(dump.Anime))
	for _, anime := range dump.Anime {
		if anime.AID == 0 {
			continue
		}
		entry := titleEntry{aid: strconv.Itoa(anime.AID)}
		for _, title := range anime.Titles {
			typ := strings.ToLower(strings.TrimSpace(title.Type))
			if typ != "main" && typ != "official" && typ != "synonym" {
				continue
			}
			raw := strings.TrimSpace(title.Value)
			if raw == "" {
				continue
			}
			entry.titles = append(entry.titles, normalizeTitle(raw))
		}
		if len(entry.titles) == 0 {
			continue
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

func (idx *titlesIndex) search(query string) (string, provider.MatchStatus) {
	needle := normalizeTitle(query)
	if needle == "" {
		return "", provider.MatchNone
	}

	exact := idx.collectMatches(needle, true)
	if len(exact) == 1 {
		return exact[0], provider.MatchOK
	}
	if len(exact) > 1 {
		return "", provider.MatchUncertain
	}

	contains := idx.collectMatches(needle, false)
	switch len(contains) {
	case 0:
		return "", provider.MatchNone
	case 1:
		return contains[0], provider.MatchOK
	default:
		return "", provider.MatchUncertain
	}
}

func (idx *titlesIndex) collectMatches(needle string, exactOnly bool) []string {
	var aids []string
	seen := map[string]struct{}{}
	for _, entry := range idx.entries {
		if !entry.matches(needle, exactOnly) {
			continue
		}
		if _, ok := seen[entry.aid]; ok {
			continue
		}
		seen[entry.aid] = struct{}{}
		aids = append(aids, entry.aid)
	}

	return aids
}

func (entry titleEntry) matches(needle string, exactOnly bool) bool {
	if slices.Contains(entry.titles, needle) {
		return true
	}
	if exactOnly {
		return false
	}
	for _, title := range entry.titles {
		if strings.Contains(title, needle) || strings.Contains(needle, title) {
			return true
		}
	}

	return false
}

type titlesDumpDTO struct {
	Anime []titlesAnimeDTO `xml:"anime"`
}

type titlesAnimeDTO struct {
	AID    int        `xml:"aid,attr"`
	Titles []titleDTO `xml:"title"`
}
