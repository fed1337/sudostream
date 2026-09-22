package provider

import "context"

// fakeAdapter implements all three category interfaces so tests can register a key without a
// real external API.
type fakeAdapter struct {
	key string

	showStatus MatchStatus
	showFields ShowFields
	showErr    error

	filmStatus MatchStatus
	filmFields FilmFields
	filmErr    error

	art       Art
	artStatus MatchStatus
	artErr    error

	subFile     SubtitleFile
	subStatus   MatchStatus
	subErr      error
	subCalls    int
	matchCalls  int
	posterCalls int
}

func newFakeAdapter(key string) *fakeAdapter {
	return &fakeAdapter{key: key, showStatus: MatchOK, filmStatus: MatchOK, artStatus: MatchOK, subStatus: MatchOK}
}

func (f *fakeAdapter) Key() string { return f.key }

func (f *fakeAdapter) MatchShow(
	_ context.Context,
	_ ShowHint,
) (MatchStatus, ShowFields, error) {
	f.matchCalls++

	return f.showStatus, f.showFields, f.showErr
}

func (f *fakeAdapter) MatchFilm(
	_ context.Context,
	_ FilmHint,
) (MatchStatus, FilmFields, error) {
	f.matchCalls++

	return f.filmStatus, f.filmFields, f.filmErr
}

func (f *fakeAdapter) FetchShowPoster(
	_ context.Context,
	_ ShowHint,
) (MatchStatus, Art, error) {
	f.posterCalls++

	return f.artStatus, f.art, f.artErr
}

func (f *fakeAdapter) FetchFilmPoster(
	_ context.Context,
	_ FilmHint,
) (MatchStatus, Art, error) {
	f.posterCalls++

	return f.artStatus, f.art, f.artErr
}

func (f *fakeAdapter) FetchSubtitle(
	_ context.Context,
	_ SubtitleHint,
	lang string,
) (MatchStatus, SubtitleFile, error) {
	f.subCalls++
	file := f.subFile
	if file.Lang == "" {
		file.Lang = lang
	}
	if len(file.Bytes) == 0 && f.subStatus == MatchOK && f.subErr == nil {
		file.Bytes = []byte("WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nHi\n")
	}

	return f.subStatus, file, f.subErr
}
