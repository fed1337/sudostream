package anilist

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sudoStream/internal/provider"
	"unicode"
)

const searchQuery = `query ($search: String, $perPage: Int) {
  Page(page: 1, perPage: $perPage) {
    media(search: $search, type: ANIME, sort: SEARCH_MATCH) {
      id
      title { romaji english native }
      description(asHtml: false)
      genres
      seasonYear
      startDate { year }
      studios(isMain: true) { nodes { name } }
      coverImage { extraLarge large }
    }
  }
}`

// yearTolerance allows a one-year drift between filename year and AniList release year
// (season splits and regional releases routinely differ by one).
const yearTolerance = 1

type searchResponse struct {
	Data struct {
		Page struct {
			Media []media `json:"media"`
		} `json:"Page"` //nolint:tagliatelle // AniList GraphQL field name
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

type media struct {
	ID    int `json:"id"`
	Title struct {
		Romaji  string `json:"romaji"`
		English string `json:"english"`
		Native  string `json:"native"`
	} `json:"title"`
	Description string   `json:"description"`
	Genres      []string `json:"genres"`
	SeasonYear  *int     `json:"seasonYear"`
	StartDate   struct {
		Year *int `json:"year"`
	} `json:"startDate"`
	Studios struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	} `json:"studios"`
	CoverImage coverImage `json:"coverImage"`
}

type coverImage struct {
	ExtraLarge string `json:"extraLarge"`
	Large      string `json:"large"`
}

func (c coverImage) best() string {
	if strings.TrimSpace(c.ExtraLarge) != "" {
		return c.ExtraLarge
	}

	return strings.TrimSpace(c.Large)
}

type mappedFields struct {
	title       *string
	description *string
	genres      []string
	studio      *string
	year        *int
	externalID  *string
}

var htmlTagPattern = regexp.MustCompile(`<[^>]*>`)

func (m media) titles() []string {
	return []string{m.Title.English, m.Title.Romaji, m.Title.Native}
}

func (m media) year() *int {
	if m.SeasonYear != nil {
		return m.SeasonYear
	}

	return m.StartDate.Year
}

func (m media) displayTitle() string {
	for _, candidate := range m.titles() {
		if strings.TrimSpace(candidate) != "" {
			return strings.TrimSpace(candidate)
		}
	}

	return ""
}

func (m media) fields() mappedFields {
	mapped := mappedFields{genres: m.Genres, year: m.year()}

	if title := m.displayTitle(); title != "" {
		mapped.title = &title
	}
	if description := plainDescription(m.Description); description != "" {
		mapped.description = &description
	}
	for _, studio := range m.Studios.Nodes {
		name := strings.TrimSpace(studio.Name)
		if name != "" {
			mapped.studio = &name

			break
		}
	}
	externalID := strconv.Itoa(m.ID)
	mapped.externalID = &externalID

	return mapped
}

// match runs one search and decides confidence (FI-1 L17): a single candidate whose title
// matches the query exactly is applied; ambiguity or fuzzy-only hits are skipped, never guessed.
func (p *Provider) match(
	ctx context.Context,
	title string,
	year *int,
) (provider.MatchStatus, media, error) {
	query := strings.TrimSpace(title)
	if query == "" {
		return provider.MatchNone, media{}, nil
	}

	body, err := json.Marshal(map[string]any{
		"query": searchQuery,
		"variables": map[string]any{
			"search":  query,
			"perPage": searchCandidates,
		},
	})
	if err != nil {
		return provider.MatchNone, media{}, fmt.Errorf("encode anilist query: %w", err)
	}

	payload, err := p.post(ctx, body)
	if err != nil {
		return provider.MatchNone, media{}, err
	}

	decoded, err := decodeGraphQL(payload)
	if err != nil {
		return provider.MatchNone, media{}, err
	}

	status, best := pickMatch(decoded.Data.Page.Media, query, year)

	return status, best, nil
}

func pickMatch(
	candidates []media,
	query string,
	year *int,
) (provider.MatchStatus, media) {
	if len(candidates) == 0 {
		return provider.MatchNone, media{}
	}

	wanted := normalizeTitle(query)
	exact := make([]media, 0, len(candidates))
	for _, candidate := range candidates {
		if titleMatches(candidate, wanted) {
			exact = append(exact, candidate)
		}
	}

	if len(exact) != 1 {
		// Zero exact hits means only fuzzy candidates; more than one means ambiguity.
		return provider.MatchUncertain, media{}
	}
	if !yearMatches(exact[0], year) {
		return provider.MatchUncertain, media{}
	}

	return provider.MatchOK, exact[0]
}

func titleMatches(candidate media, wanted string) bool {
	for _, title := range candidate.titles() {
		if normalizeTitle(title) == wanted && wanted != "" {
			return true
		}
	}

	return false
}

func yearMatches(candidate media, year *int) bool {
	candidateYear := candidate.year()
	if year == nil || candidateYear == nil {
		return true
	}

	diff := *year - *candidateYear
	if diff < 0 {
		diff = -diff
	}

	return diff <= yearTolerance
}

func normalizeTitle(title string) string {
	var builder strings.Builder

	for _, symbol := range strings.ToLower(title) {
		if unicode.IsLetter(symbol) || unicode.IsDigit(symbol) {
			builder.WriteRune(symbol)
		}
	}

	return builder.String()
}

func plainDescription(raw string) string {
	cleaned := strings.ReplaceAll(raw, "<br>", "\n")
	cleaned = strings.ReplaceAll(cleaned, "<br/>", "\n")
	cleaned = strings.ReplaceAll(cleaned, "<br />", "\n")
	cleaned = htmlTagPattern.ReplaceAllString(cleaned, "")
	cleaned = strings.ReplaceAll(cleaned, "&amp;", "&")
	cleaned = strings.ReplaceAll(cleaned, "&quot;", `"`)
	cleaned = strings.ReplaceAll(cleaned, "&#039;", "'")

	return strings.TrimSpace(cleaned)
}
