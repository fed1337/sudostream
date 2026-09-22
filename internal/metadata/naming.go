package metadata

import (
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

//nolint:gochecknoglobals // compiled once
var (
	// S01E02 / S5EP02 / S2ep12 (optional P after E).
	episodePattern = regexp.MustCompile(
		`(?i)(?:^|[\s._-])[s](\d{1,2})[\s._-]*e(?:p)?(\d{1,3})(?:[\s._-]|$)`,
	)
	episodeDotPattern = regexp.MustCompile(
		`(?i)(?:^|[\s._-])[s](\d{1,2})\.e(\d{1,3})(?:[\s._-]|$)`,
	)
	episodeRangePattern = regexp.MustCompile(
		`(?i)[s](\d{1,2})[\s._-]*[e](\d{1,3})\s*-\s*[e]?(\d{1,3})`,
	)
	episodeXPattern = regexp.MustCompile(
		`(?i)(?:^|[\s._-])(\d{1,2})[xх](\d{1,3})(?:[\s._-]|$)`,
	)
	// "Сезон 06. Серия 120" / "Season 1 Episode 8" in the same basename.
	// Note: Go (?i)/\b are ASCII-oriented; Cyrillic needs explicit case + non-\b ends.
	seasonEpisodeWordsPattern = regexp.MustCompile(
		`(?i)(?:season|[Сс]езон)[\s._№#:-]*(\d{1,2})(?:[^\p{L}\p{N}]|$).*?` +
			`(?:episode|[Сс]ерия|seriya)[\s._№#:-]*(\d{1,3})(?:[^\p{L}\p{N}]|$)`,
	)
	// Episode-only: "Серия №219", "(18.seriya)", bare "e7" / "show07".
	episodeWordsPattern = regexp.MustCompile(
		`(?i)(?:^|[\s._(-])(?:episode|[Сс]ерия|seriya|ep)[\s._№#:-]*(\d{1,3})(?:[^\p{L}\p{N}]|$)`,
	)
	episodeParenSeriyaPattern = regexp.MustCompile(
		`(?i)\((\d{1,3})\s*\.?\s*(?:seriya|[Сс]ерия|ep(?:isode)?)\)`,
	)
	episodeParenOfPattern = regexp.MustCompile(
		`(?i)\((\d{1,3})\s*\.?\s*(?:iz|of)[\s.]*\d{1,3}\)`,
	)
	bareEpisodeMarkerPattern = regexp.MustCompile(
		`(?i)^(?:e|ep|show)[\s._-]*(\d{1,3})$`,
	)
	// "Season 2", "[Season 2]", "Сезон №11", "2 Season", "4 Сезон".
	seasonFolderPattern = regexp.MustCompile(
		`(?i)(?:^|[\s._\[(-])(?:season|[Сс]езон)[\s._№#:-]*(\d{1,2})(?:[^\p{L}\p{N}]|$)`,
	)
	seasonFolderTrailingPattern = regexp.MustCompile(
		`(?i)(?:^|[\s._\[(-])(\d{1,2})[\s._-]*(?:season|[Сс]езон)(?:[^\p{L}\p{N}]|$)`,
	)
	seasonPackPattern = regexp.MustCompile(
		`(?i)(?:^|[\s._-])[s](\d{1,2})(?:[\s._-]|$)`,
	)
	seriesRangeJunkPattern = regexp.MustCompile(
		`(?i)(?:^|[\s._-])(?:[Сс]ерии|episodes?)[\s._№#:-]*\d{1,3}\s*[-–—]\s*\d{1,3}(?:[^\p{L}\p{N}]|$)`,
	)
	// "Seasons 1 to 3", "(63 serij.iz.63)", "1957-2002".
	seasonsSpanJunkPattern = regexp.MustCompile(
		`(?i)(?:^|[\s._-])seasons?[\s._-]*\d{1,2}[\s._-]*(?:to|through|[-–—])[\s._-]*\d{1,2}` +
			`(?:[^\p{L}\p{N}]|$)`,
	)
	seriiCountJunkPattern = regexp.MustCompile(
		`(?i)[(\[]?\d{1,3}\s*[.\s_-]*(?:serii|serij|[Сс]ери[йия])\s*[.\s_-]*(?:iz|из|of)` +
			`\s*[.\s_-]*\d{1,3}[)\]]?`,
	)
	yearRangePattern = regexp.MustCompile(
		`(?:^|[\s._(-])((?:19|20)\d{2})\s*[-–—]\s*((?:19|20)\d{2})(?:[\s._)\]]|$)`,
	)
	codecJunkPattern = regexp.MustCompile(
		`(?i)(?:^|[\s._-])h\.?26[45](?:[\s._-]|$)`,
	)
	// Leftover "Title. (" after season stripping (RE2 has no lookahead).
	orphanDotBeforeParenPattern = regexp.MustCompile(`\.\s*\(`)
	multiDotPattern             = regexp.MustCompile(`\.{2,}`)
	// Scene separators: "Foo.Bar" → space; keep title punctuation "Foo. Bar".
	sceneDotPattern   = regexp.MustCompile(`(\S)\.(\S)`)
	yearParenPattern  = regexp.MustCompile(`\((19|20)\d{2}\)`)
	yearDotPattern    = regexp.MustCompile(`(?:^|[\s._-])((?:19|20)\d{2})(?:[\s._-]|$)`)
	absoluteEpPattern = regexp.MustCompile(`(?i)^(\d{1,3})$`)
	// Trailing style: "Title - 01", "Title - 01v2", "Title - 01 END" before [tags].
	absoluteEpCutPattern = regexp.MustCompile(
		`(?i)[\s._-]+(\d{1,3})(?:\s*v\d+)?(?:\s*(?:end|ova|ona|special))?` +
			`(?:\s*\[[^\]]*])*` +
			`(?:\s*\{[^}]*})*` +
			`\s*$`,
	)
	// Mid style: "Show - 025 - Episode Title [tags]" (title must not start with '[' / '{').
	absoluteEpWithTitlePattern = regexp.MustCompile(
		`(?i)^(.+?)([\s._-]+)(\d{1,3})(?:\s*v\d+)?([\s._-]+)([^\[\{].*?)` +
			`(?:\s*\[[^\]]*])*` +
			`(?:\s*\{[^}]*})*` +
			`\s*$`,
	)
	// Leading style: "23_Krtek_a_buldozer_(1975)" / "22.Title.(1975)".
	leadingAbsoluteEpPattern = regexp.MustCompile(
		`(?i)^(\d{1,3})([._\s-]+)(.+)$`,
	)
	// Mid "(43)" absolute episode token (not a year).
	parenAbsoluteEpPattern = regexp.MustCompile(
		`(?i)(?:^|[\s._-])\((\d{1,3})\)(?:[\s._-]|$)`,
	)
	bracketTagPattern = regexp.MustCompile(`\[[^\]]*]|\{[^}]*}`)
	digitsOnlyPattern = regexp.MustCompile(`^\d{1,4}$`)
)

// Quality / source tokens stripped from titles (lowercase compare).
var junkTokens = map[string]struct{}{ //nolint:gochecknoglobals // static set
	"1080p": {}, "720p": {}, "480p": {}, "2160p": {}, "4k": {}, "uhd": {},
	"web-dl": {}, "webdl": {}, "webrip": {}, "bdrip": {}, "bluray": {}, "blu": {},
	"ray": {}, "hdtv": {}, "hdrip": {}, "dvdrip": {}, "dvd": {}, "brrip": {},
	"x264": {}, "x265": {}, "h264": {}, "h265": {}, "hevc": {}, "avc": {},
	"10bit": {}, "8bit": {}, "aac": {}, "ac3": {}, "dts": {}, "truehd": {},
	"atmos": {}, "remux": {}, "proper": {}, "repack": {}, "extended": {},
	"unrated": {}, "directors": {}, "cut": {}, "open": {}, "matte": {},
	"complete": {}, "series": {}, "season": {}, "seasons": {}, "episode": {},
	"mp4": {}, "mkv": {}, "avi": {}, "rus": {}, "eng": {}, "multi": {},
	"sub": {}, "subs": {}, "internal": {}, "limited": {}, "readnfo": {},
	"amzn": {}, "nf": {}, "dsnp": {}, "it": {}, "web": {}, "hdclub": {},
	"lostfilm": {}, "hd": {}, "sd": {}, "hdr": {}, "sdr": {}, "dl": {},
	"sitcom": {}, "hdtvrip": {}, "satrip": {}, "xvid": {}, "eniahd": {},
	"teamhd": {}, "exkinoray": {},
}

// FilmIdentity is a heuristic film title/year from a path.
type FilmIdentity struct {
	Title string
	Year  *int
}

// SeriesIdentity is a heuristic show/season/episode from a path.
type SeriesIdentity struct {
	Show         string
	ShowKey      string
	Season       *int
	Episode      *int
	EpisodeTitle string
}

// NormalizeShowKey builds a stable grouping key from a show display name.
func NormalizeShowKey(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ""
	}

	var builder strings.Builder
	prevDash := false
	for _, runeValue := range name {
		if unicode.IsLetter(runeValue) || unicode.IsDigit(runeValue) {
			builder.WriteRune(runeValue)
			prevDash = false

			continue
		}
		if !prevDash && builder.Len() > 0 {
			builder.WriteByte('-')
			prevDash = true
		}
	}

	return strings.Trim(builder.String(), "-")
}

// ParseFilmIdentity extracts title/year from a media relative path.
func ParseFilmIdentity(relPath string) FilmIdentity {
	basename := strings.TrimSuffix(filepath.Base(relPath), filepath.Ext(relPath))
	title, year := cleanTitleAndYear(basename)
	if title == "" {
		title = basename
	}

	return FilmIdentity{Title: title, Year: year}
}

// ParseSeriesIdentity extracts show/season/episode from path + basename.
func ParseSeriesIdentity(relPath string) SeriesIdentity {
	relPath = filepath.ToSlash(relPath)
	basename := strings.TrimSuffix(filepath.Base(relPath), filepath.Ext(relPath))
	parent, grandparent := pathParents(relPath)

	season, episode, episodeTitle, abs := resolveSeasonEpisode(basename, parent)
	show := deriveShowName(basename, parent, grandparent, abs)
	showKey := NormalizeShowKey(show)

	return SeriesIdentity{
		Show:         show,
		ShowKey:      showKey,
		Season:       season,
		Episode:      episode,
		EpisodeTitle: episodeTitle,
	}
}

func pathParents(relPath string) (string, string) {
	parentDir := filepath.Dir(relPath)
	parent := filepath.Base(parentDir)
	if parent == "." || parent == "/" {
		parent = ""
	}
	grandparent := filepath.Base(filepath.Dir(parentDir))
	if grandparent == "." || grandparent == "/" || grandparent == parent {
		grandparent = ""
	}

	return parent, grandparent
}

func resolveSeasonEpisode(
	basename, parent string,
) (*int, *int, string, *absoluteEpisodeInfo) {
	season, episode := parseSeasonEpisode(basename)
	if season == nil {
		season = parseSeasonFromFolder(parent)
	}
	if episode == nil {
		episode = parseEpisodeOnly(basename)
	}
	if episode == nil && season != nil {
		episode = parseSEEEpisode(basename, *season)
	}
	if season == nil && episode != nil {
		seasonOne := 1
		season = &seasonOne
	}
	var abs *absoluteEpisodeInfo
	var episodeTitle string
	if season == nil && episode == nil {
		abs = parseAbsoluteEpisodeInfo(basename)
		if abs != nil {
			seasonOne := 1
			season = &seasonOne
			episode = &abs.Number
			episodeTitle = abs.Title
		}
	}

	return season, episode, episodeTitle, abs
}

// ParseFilenameHint extracts season/episode from a basename (no extension).
func ParseFilenameHint(basename string) *FilenameHint {
	season, episode := parseSeasonEpisode(basename)
	if season == nil || episode == nil {
		return nil
	}

	return &FilenameHint{Season: season, Episode: episode}
}

//nolint:mnd // regexp capture group counts
func parseSeasonEpisode(basename string) (*int, *int) {
	if match := episodeRangePattern.FindStringSubmatch(basename); len(match) >= 3 {
		return atoiPtr(match[1]), atoiPtr(match[2])
	}
	if match := seasonEpisodeWordsPattern.FindStringSubmatch(basename); len(match) == 3 {
		return atoiPtr(match[1]), atoiPtr(match[2])
	}
	if match := episodePattern.FindStringSubmatch(basename); len(match) == 3 {
		return atoiPtr(match[1]), atoiPtr(match[2])
	}
	if match := episodeDotPattern.FindStringSubmatch(basename); len(match) == 3 {
		return atoiPtr(match[1]), atoiPtr(match[2])
	}
	if match := episodeXPattern.FindStringSubmatch(basename); len(match) == 3 {
		return atoiPtr(match[1]), atoiPtr(match[2])
	}

	return nil, nil
}

//nolint:mnd // regexp capture group counts
func parseEpisodeOnly(basename string) *int {
	if match := episodeParenSeriyaPattern.FindStringSubmatch(basename); len(match) == 2 {
		return atoiPtr(match[1])
	}
	if match := episodeParenOfPattern.FindStringSubmatch(basename); len(match) == 2 {
		return atoiPtr(match[1])
	}
	if match := episodeWordsPattern.FindStringSubmatch(basename); len(match) == 2 {
		return atoiPtr(match[1])
	}
	if match := bareEpisodeMarkerPattern.FindStringSubmatch(basename); len(match) == 2 {
		return atoiPtr(match[1])
	}

	return nil
}

// parseSEEEpisode decodes Season/Episode packing like "208" → E08 when season is 2.
func parseSEEEpisode(basename string, season int) *int {
	const seeModulus = 100
	if season <= 0 || !digitsOnlyPattern.MatchString(basename) {
		return nil
	}
	value, err := strconv.Atoi(basename)
	if err != nil || value <= 0 {
		return nil
	}
	switch {
	case value >= season*seeModulus && value < (season+1)*seeModulus:
		episode := value % seeModulus
		if episode <= 0 {
			return nil
		}

		return &episode
	case value < seeModulus:
		// Bare episode under a known season folder: "06", "44".
		return &value
	default:
		return nil
	}
}

//nolint:mnd // regexp capture group counts
func parseSeasonFromFolder(folder string) *int {
	if folder == "" {
		return nil
	}
	if match := seasonFolderPattern.FindStringSubmatch(folder); len(match) == 2 {
		return atoiPtr(match[1])
	}
	if match := seasonFolderTrailingPattern.FindStringSubmatch(folder); len(match) == 2 {
		return atoiPtr(match[1])
	}
	if match := seasonPackPattern.FindStringSubmatch(folder); len(match) == 2 {
		return atoiPtr(match[1])
	}

	return nil
}

type absoluteEpisodeInfo struct {
	Number int
	Title  string
	CutAt  int
}

func parseAbsoluteEpisodeInfo(basename string) *absoluteEpisodeInfo {
	if info := parseMidAbsoluteEpisode(basename); info != nil {
		return info
	}
	if info := parseLeadingAbsoluteEpisode(basename); info != nil {
		return info
	}
	if info := parseParenAbsoluteEpisode(basename); info != nil {
		return info
	}
	if info := parseTrailingAbsoluteEpisode(basename); info != nil {
		return info
	}

	return parseTrailingTokenAbsoluteEpisode(basename)
}

func parseParenAbsoluteEpisode(basename string) *absoluteEpisodeInfo {
	match := parenAbsoluteEpPattern.FindStringSubmatch(basename)
	if len(match) != 2 { //nolint:mnd // regexp capture count
		return nil
	}
	episodeNum := episodeNumberFromToken(match[1])
	if episodeNum == nil {
		return nil
	}

	return &absoluteEpisodeInfo{Number: *episodeNum, CutAt: -1}
}

func parseMidAbsoluteEpisode(basename string) *absoluteEpisodeInfo {
	match := absoluteEpWithTitlePattern.FindStringSubmatchIndex(basename)
	if match == nil {
		return nil
	}
	episodeNum := episodeNumberFromToken(basename[match[6]:match[7]])
	if episodeNum == nil {
		return nil
	}
	title, _ := cleanTitleAndYear(strings.TrimSpace(basename[match[10]:match[11]]))

	return &absoluteEpisodeInfo{
		Number: *episodeNum,
		Title:  title,
		CutAt:  match[4],
	}
}

func parseLeadingAbsoluteEpisode(basename string) *absoluteEpisodeInfo {
	match := leadingAbsoluteEpPattern.FindStringSubmatchIndex(basename)
	if match == nil {
		return nil
	}
	episodeNum := episodeNumberFromToken(basename[match[2]:match[3]])
	rest := strings.TrimSpace(basename[match[6]:match[7]])
	if episodeNum == nil || rest == "" {
		return nil
	}
	title, _ := cleanTitleAndYear(rest)
	if title == "" {
		return nil
	}

	return &absoluteEpisodeInfo{Number: *episodeNum, Title: title, CutAt: 0}
}

func parseTrailingAbsoluteEpisode(basename string) *absoluteEpisodeInfo {
	match := absoluteEpCutPattern.FindStringSubmatchIndex(basename)
	if match == nil {
		return nil
	}
	episodeNum := episodeNumberFromToken(basename[match[2]:match[3]])
	if episodeNum == nil {
		return nil
	}

	return &absoluteEpisodeInfo{Number: *episodeNum, CutAt: match[0]}
}

func parseTrailingTokenAbsoluteEpisode(basename string) *absoluteEpisodeInfo {
	cleaned := bracketTagPattern.ReplaceAllString(basename, " ")
	parts := splitNameTokens(cleaned)
	for _, token := range slices.Backward(parts) {
		lower := strings.ToLower(token)
		if _, junk := junkTokens[lower]; junk {
			continue
		}
		if episodeNum := episodeNumberFromToken(token); episodeNum != nil {
			return &absoluteEpisodeInfo{Number: *episodeNum, CutAt: -1}
		}
		// Hit a non-junk word while scanning from the end — stop (number should be near end).
		break
	}

	return nil
}

func episodeNumberFromToken(token string) *int {
	if !absoluteEpPattern.MatchString(token) {
		return nil
	}
	value, err := strconv.Atoi(token)
	if err != nil || value <= 0 || value > 999 {
		return nil
	}
	// Reject years mistaken as episode numbers.
	if value >= 1900 && value <= 2099 {
		return nil
	}

	return &value
}

func deriveShowName(basename, parent, grandparent string, abs *absoluteEpisodeInfo) string {
	// Prefer folder structure: season dirs yield the show root above them;
	// non-season parents are the show (including spin-offs under a collection).
	if name := showNameFromFolder(parent); name != "" {
		return name
	}
	if isSeasonOnlyFolder(parent) {
		if name := showNameFromFolder(grandparent); name != "" {
			return name
		}
	}

	if fromFile := showPrefixFromBasename(basename); fromFile != "" {
		return fromFile
	}
	if abs != nil && abs.Title != "" {
		if hint := firstTitleToken(abs.Title); hint != "" {
			return hint
		}

		return abs.Title
	}
	if name := cleanLooseFolderName(grandparent); name != "" {
		return name
	}
	title, _ := cleanTitleAndYear(basename)
	if title != "" {
		return title
	}

	return basename
}

func showNameFromFolder(folder string) string {
	if folder == "" || isSeasonOnlyFolder(folder) {
		return ""
	}
	if name := cleanShowFolder(folder); name != "" {
		return name
	}

	return cleanLooseFolderName(folder)
}

func isSeasonOnlyFolder(folder string) bool {
	if folder == "" || parseSeasonFromFolder(folder) == nil {
		return false
	}

	return cleanShowFolder(folder) == ""
}

func cleanLooseFolderName(folder string) string {
	if folder == "" {
		return ""
	}
	title := cleanShowTitle(folder)
	if title != "" {
		return title
	}

	return folder
}

func firstTitleToken(title string) string {
	tokens := strings.Fields(title)
	const minTokensForShowHint = 2
	if len(tokens) < minTokensForShowHint {
		return ""
	}

	return tokens[0]
}

func showPrefixFromBasename(basename string) string {
	lower := strings.ToLower(basename)
	cut := -1
	for _, pattern := range []*regexp.Regexp{
		episodeRangePattern,
		seasonEpisodeWordsPattern,
		episodePattern,
		episodeDotPattern,
		episodeXPattern,
		episodeWordsPattern,
		episodeParenSeriyaPattern,
		episodeParenOfPattern,
	} {
		loc := pattern.FindStringIndex(lower)
		if loc != nil && (cut < 0 || loc[0] < cut) {
			cut = loc[0]
		}
	}
	if abs := parseAbsoluteEpisodeInfo(basename); abs != nil && abs.CutAt > 0 &&
		(cut < 0 || abs.CutAt < cut) {
		cut = abs.CutAt
	}
	if cut <= 0 {
		return ""
	}

	prefix := strings.Trim(basename[:cut], " ._-+")

	return cleanShowTitle(prefix)
}

func cleanShowFolder(folder string) string {
	if folder == "" {
		return ""
	}
	cleaned := seasonFolderPattern.ReplaceAllString(folder, " ")
	cleaned = seasonFolderTrailingPattern.ReplaceAllString(cleaned, " ")
	cleaned = seasonPackPattern.ReplaceAllString(cleaned, " ")
	cleaned = seriesRangeJunkPattern.ReplaceAllString(cleaned, " ")
	cleaned = seasonsSpanJunkPattern.ReplaceAllString(cleaned, " ")

	return cleanShowTitle(cleaned)
}

// cleanShowTitle strips release/pack junk while keeping title punctuation like "Универ. Новая".
func cleanShowTitle(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	raw = seriiCountJunkPattern.ReplaceAllString(raw, " ")
	raw = codecJunkPattern.ReplaceAllString(raw, " ")
	raw = yearRangePattern.ReplaceAllString(raw, " ")
	raw = yearParenPattern.ReplaceAllString(raw, " ")
	raw = yearDotPattern.ReplaceAllString(raw, " ")
	raw = bracketTagPattern.ReplaceAllString(raw, " ")
	raw = orphanDotBeforeParenPattern.ReplaceAllString(raw, " (")
	raw = multiDotPattern.ReplaceAllString(raw, ".")
	raw = strings.TrimRight(raw, ". \t")
	raw = sceneDotPattern.ReplaceAllString(raw, "$1 $2")
	raw = strings.ReplaceAll(raw, "_", " ")
	raw = strings.ReplaceAll(raw, "-", " ")

	tokens := strings.Fields(raw)
	kept := make([]string, 0, len(tokens))
	for _, token := range tokens {
		lower := strings.ToLower(strings.Trim(token, "()[]{},"))
		if lower == "" {
			continue
		}
		if _, junk := junkTokens[lower]; junk {
			continue
		}
		if yearDotPattern.MatchString(token) {
			continue
		}
		kept = append(kept, token)
	}
	if len(kept) == 0 {
		return ""
	}

	return strings.TrimRight(strings.Join(kept, " "), ". ")
}

//nolint:mnd // year regex group counts
func cleanTitleAndYear(raw string) (string, *int) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}

	var year *int
	if match := yearParenPattern.FindStringSubmatch(raw); len(match) > 0 {
		year = atoiPtr(strings.Trim(match[0], "()"))
		raw = yearParenPattern.ReplaceAllString(raw, " ")
	} else if match := yearDotPattern.FindStringSubmatch(raw); len(match) == 2 {
		year = atoiPtr(match[1])
		raw = yearDotPattern.ReplaceAllString(raw, " ")
	}

	// Drop bracketed release tags.
	raw = bracketTagPattern.ReplaceAllString(raw, " ")

	tokens := splitNameTokens(raw)
	kept := make([]string, 0, len(tokens))
	for _, token := range tokens {
		lower := strings.ToLower(token)
		if _, junk := junkTokens[lower]; junk {
			continue
		}
		if yearDotPattern.MatchString(token) {
			continue
		}
		kept = append(kept, token)
	}
	if len(kept) == 0 {
		return "", year
	}

	title := strings.Join(kept, " ")
	title = strings.Join(strings.Fields(title), " ")

	return title, year
}

func splitNameTokens(raw string) []string {
	raw = strings.ReplaceAll(raw, ".", " ")
	raw = strings.ReplaceAll(raw, "_", " ")
	raw = strings.ReplaceAll(raw, "-", " ")
	parts := strings.Fields(raw)
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}

	return out
}

func atoiPtr(raw string) *int {
	value, err := strconv.Atoi(raw)
	if err != nil {
		return nil
	}

	return &value
}
