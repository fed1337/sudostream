package metadata

import (
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestParseFilmIdentity(t *testing.T) {
	t.Parallel()

	allure.Test(t, "parses title and year from common torrent names", func(a *allure.Context) {
		t := a.T()
		cases := []struct {
			path      string
			wantTitle string
			wantYear  int
		}{
			{"Movies/Casino.1995.BDRip.1080p.mkv", "Casino", 1995},
			{"Movies/Borderlands (2024) WEB-DL.1080p.mkv", "Borderlands", 2024},
			{"Movies/Blow.2001.BDRip.720p.mkv", "Blow", 2001},
		}
		for _, testCase := range cases {
			got := ParseFilmIdentity(testCase.path)
			if got.Title != testCase.wantTitle {
				t.Fatalf("%s title=%q want %q", testCase.path, got.Title, testCase.wantTitle)
			}
			if got.Year == nil || *got.Year != testCase.wantYear {
				t.Fatalf("%s year=%v want %d", testCase.path, got.Year, testCase.wantYear)
			}
		}
	})
}

//nolint:cyclop // fixture assertions
func TestParseSeriesIdentity_GroupsSeasonFolders(
	t *testing.T,
) {
	t.Parallel()

	allure.Test(t, "season folders normalize to same show key", func(a *allure.Context) {
		t := a.T()
		first := ParseSeriesIdentity(
			"series/90210 2008 Season 1 Complete WEB x264/90210 S01E01 Pilot.mkv",
		)
		second := ParseSeriesIdentity(
			"series/90210 2008 Season 2 Complete WEB x264/90210 S02E03 Episode.mkv",
		)
		if first.ShowKey == "" || first.ShowKey != second.ShowKey {
			t.Fatalf("show keys %q vs %q", first.ShowKey, second.ShowKey)
		}
		if first.Season == nil || *first.Season != 1 || first.Episode == nil ||
			*first.Episode != 1 {
			t.Fatalf("season/ep first=%v/%v", first.Season, first.Episode)
		}
		if second.Season == nil || *second.Season != 2 || second.Episode == nil ||
			*second.Episode != 3 {
			t.Fatalf("season/ep second=%v/%v", second.Season, second.Episode)
		}
	})
}

func TestParseFilenameHint_ExtendedPatterns(t *testing.T) {
	t.Parallel()

	allure.Test(t, "parses SxxEyy, SxxEPyy, S01.E01, and NxNN", func(a *allure.Context) {
		t := a.T()
		cases := []struct {
			name        string
			wantSeason  int
			wantEpisode int
			wantNil     bool
		}{
			{"Show.S01E02.Title", 1, 2, false},
			{"Show.S01.E03.2023", 1, 3, false},
			{"Melrose Place 1х01 - Pilot", 1, 1, false},
			{"Show.S01E01-E02", 1, 1, false},
			{"S5EP02 Red Shoe Diaries Carried Away", 5, 2, false},
			{"S2ep12 Like Father, Unlike Son", 2, 12, false},
			{"Show Name Only", 0, 0, true},
		}
		for _, testCase := range cases {
			hint := ParseFilenameHint(testCase.name)
			if testCase.wantNil {
				if hint != nil {
					t.Fatalf("%s: want nil got %+v", testCase.name, hint)
				}

				continue
			}
			if hint == nil || *hint.Season != testCase.wantSeason ||
				*hint.Episode != testCase.wantEpisode {
				t.Fatalf(
					"%s: %+v want S%dE%d",
					testCase.name,
					hint,
					testCase.wantSeason,
					testCase.wantEpisode,
				)
			}
		}
	})
}

func TestParseSeriesIdentity_AbsoluteAnimeStyle(t *testing.T) {
	t.Parallel()

	const krtekShow = "Krtek"

	allure.Test(t, "parses absolute episode numbers including leading NN_Title_(year)", func(a *allure.Context) {
		t := a.T()
		cases := []struct {
			path             string
			wantEpisode      int
			wantShow         string
			wantEpisodeTitle string
		}{
			{
				path:        "[SubsPlease] Sono Bisque Doll wa Koi wo Suru - 01 [1080p].mkv",
				wantEpisode: 1,
				wantShow:    "Sono Bisque Doll wa Koi wo Suru",
			},
			{
				path:        "anime/My Dress-Up Darling/[SubsPlease] Sono Bisque Doll wa Koi wo Suru - 12 [1080p].mkv",
				wantEpisode: 12,
				wantShow:    "My Dress Up Darling",
			},
			{
				path:        "Show Name - 03v2 [1080p][AAC].mkv",
				wantEpisode: 3,
				wantShow:    "Show Name",
			},
			{
				path:             "Chilly Willy - 025 - South Pole Pals [SATRip-Rus].avi",
				wantEpisode:      25,
				wantShow:         "Chilly Willy",
				wantEpisodeTitle: "South Pole Pals",
			},
			{
				path:             "22_Krtek_hodinarem_(1975).avi",
				wantEpisode:      22,
				wantShow:         krtekShow,
				wantEpisodeTitle: "Krtek hodinarem",
			},
			{
				path:             "23_Krtek_a_buldozeк_(1975).avi",
				wantEpisode:      23,
				wantShow:         krtekShow,
				wantEpisodeTitle: "Krtek a buldozeк",
			},
			{
				path:             "shows/Krtek/23_Krtek_a_buldozeк_(1975).avi",
				wantEpisode:      23,
				wantShow:         krtekShow,
				wantEpisodeTitle: "Krtek a buldozeк",
			},
		}
		for _, testCase := range cases {
			got := ParseSeriesIdentity(testCase.path)
			if got.Episode == nil || *got.Episode != testCase.wantEpisode {
				t.Fatalf("%s episode=%v want %d", testCase.path, got.Episode, testCase.wantEpisode)
			}
			if got.Season == nil || *got.Season != 1 {
				t.Fatalf("%s season=%v want 1", testCase.path, got.Season)
			}
			if got.Show != testCase.wantShow {
				t.Fatalf("%s show=%q want %q", testCase.path, got.Show, testCase.wantShow)
			}
			if got.EpisodeTitle != testCase.wantEpisodeTitle {
				t.Fatalf(
					"%s episodeTitle=%q want %q",
					testCase.path,
					got.EpisodeTitle,
					testCase.wantEpisodeTitle,
				)
			}
		}
	})
}

type seriesParseCase struct {
	path    string
	season  int
	episode int
	show    string
}

func assertSeriesParseCases(t *testing.T, cases []seriesParseCase) {
	t.Helper()
	for _, testCase := range cases {
		got := ParseSeriesIdentity(testCase.path)
		if got.Season == nil || *got.Season != testCase.season {
			t.Fatalf("%s season=%v want %d", testCase.path, got.Season, testCase.season)
		}
		if got.Episode == nil || *got.Episode != testCase.episode {
			t.Fatalf("%s episode=%v want %d", testCase.path, got.Episode, testCase.episode)
		}
		if testCase.show != "" && got.Show != testCase.show {
			t.Fatalf("%s show=%q want %q", testCase.path, got.Show, testCase.show)
		}
	}
}

func TestParseSeriesIdentity_RealLibraryNameStyles(t *testing.T) {
	t.Parallel()

	allure.Test(t, "parses season/episode from mixed real library path styles", func(a *allure.Context) {
		assertSeriesParseCases(a.T(), realLibrarySeriesCases())
	})
}

func TestParseSeriesIdentity_FolderStructureGroupsSeasons(t *testing.T) {
	t.Parallel()

	allure.Test(t, "season folders under one show root share ShowKey; spin-offs do not", func(a *allure.Context) {
		t := a.T()
		const univerShow = "Универ. Новая общага"
		seasonSix := ParseSeriesIdentity(
			"series/Универ. Новая общага/Универ. Новая общага. Сезон 06 (2014)/" +
				"Универ. Новая общага. Сезон 06. Серия 120.avi",
		)
		seasonOne := ParseSeriesIdentity(
			"series/Универ. Новая общага/Универ. Новая общага. Сезон 01 (2011)/" +
				"Универ. Новая общага. Сезон 01. Серия 008.avi",
		)
		assertShowSeason(t, "univer-s06", seasonSix, univerShow, 6)
		assertShowSeason(t, "univer-s01", seasonOne, univerShow, 1)
		if seasonSix.ShowKey == "" || seasonSix.ShowKey != seasonOne.ShowKey {
			t.Fatalf("ShowKey s06=%q s01=%q", seasonSix.ShowKey, seasonOne.ShowKey)
		}

		comedyShow := ParseSeriesIdentity(
			"cartoons/Tom and Jerry (The exotic сollection)/Том и Джерри Комедийное Шоу/e7.avi",
		)
		newShow := ParseSeriesIdentity(
			"cartoons/Tom and Jerry (The exotic сollection)/Новое шоу Тома и Джерри/show07.avi",
		)
		assertShowSeason(t, "tj-comedy", comedyShow, "Том и Джерри Комедийное Шоу", 1)
		assertShowSeason(t, "tj-new", newShow, "Новое шоу Тома и Джерри", 1)
		if comedyShow.ShowKey == newShow.ShowKey {
			t.Fatalf("spin-offs must not share ShowKey: %q", comedyShow.ShowKey)
		}

		billionsS3 := ParseSeriesIdentity(
			"series/Billions Seasons 1 to 3 Mp4 1080p/Season 3/Billions S03E01.mp4",
		)
		billionsS2 := ParseSeriesIdentity(
			"series/Billions Seasons 1 to 3 Mp4 1080p/Season 2/Billions S02E05.mp4",
		)
		assertShowSeason(t, "billions-s3", billionsS3, "Billions", 3)
		assertShowSeason(t, "billions-s2", billionsS2, "Billions", 2)
		if billionsS3.ShowKey != billionsS2.ShowKey {
			t.Fatalf("billions ShowKey %q vs %q", billionsS3.ShowKey, billionsS2.ShowKey)
		}
	})
}

func assertShowSeason(
	t *testing.T,
	label string,
	got SeriesIdentity,
	wantShow string,
	wantSeason int,
) {
	t.Helper()
	if got.Show != wantShow {
		t.Fatalf("%s show=%q want %q", label, got.Show, wantShow)
	}
	if got.Season == nil || *got.Season != wantSeason {
		t.Fatalf("%s season=%v want %d", label, got.Season, wantSeason)
	}
}

//nolint:funlen // representative fixtures from a real library dump
func realLibrarySeriesCases() []seriesParseCase {
	const interny = "Интерны"
	const pimpUK = "Pimp my Ride UK"

	return []seriesParseCase{
		// SxxEyy / scene
		{
			"series/BH90210.S01.AMZN.WEB-DL.720p.LostFilm/" +
				"BH.90120.S01E01.The.Reunion.AMZN.WEB-DL.720p.mkv",
			1, 1, "BH90210",
		},
		{
			"series/The.Penguin.S01.1080p.AMZN.WEB-DL.H.264-EniaHD/" +
				"The.Penguin.S01E01.After.Hours.1080p.AMZN.WEB-DL.H.264-EniaHD.mkv",
			1, 1, "The Penguin",
		},
		{
			"series/Queer As Folk US Season 1 Complete 720P AMZN WEB-DL x264 [i_c]/" +
				"Queer as Folk (US) S01E03 No Bris, No Shirt, No Service.mkv",
			1, 3, "Queer As Folk US",
		},
		{
			"series/Billions Seasons 1 to 3 Mp4 1080p/Season 3/Billions S03E01.mp4",
			3, 1, "Billions",
		},
		{
			"series/Slovo.pacana.Krov.na.asfalte.S01.2023.WEB-DL.1080p/" +
				"Slovo.pacana.Krov.na.asfalte.S01.E04.2023.WEB-DL.1080p.mkv",
			1, 4, "Slovo pacana Krov na asfalte",
		},
		{
			"series/Univer.10.let.spustya.S01.2021.WEB-DL.1080p/" +
				"Univer.10.let.spustya.S01.E10.2021.WEB-DL.1080p.mkv",
			1, 10, "Univer 10 let spustya",
		},
		{
			"cartoons/Arcane.S02.1080p.NF.WEB-DL.x264-EniaHD/" +
				"Arcane.S02E04.Paint.the.Town.Blue.1080p.NF.WEB-DL.x264-EniaHD.mkv",
			2, 4, "Arcane",
		},
		{
			"tv/Life After People Season 02/Life.After.People.s02e10.720p.mkv",
			2, 10, "Life After People",
		},
		{"tv/room_raiders/Room_raiders_S04E22_(Evan)_by_[Nifer].avi", 4, 22, "room raiders"},
		{"tv/Секс с Анфисой Чеховой/4 Сезон/s04e20.flv", 4, 20, "Секс с Анфисой Чеховой"},
		// SxEPyy / Sxepyy
		{
			"series/Red Shoe Diaries/Season 5/S5EP02 Red Shoe Diaries Carried Away.mkv",
			5, 2, "Red Shoe Diaries",
		},
		{
			"cartoons/The New Woody Woodpecker Show (1999-2000)/2 Season/" +
				"S2ep12 Like Father, Unlike Son - A Chilly Spy - Country Fair Clam-ity.mkv",
			2, 12, "The New Woody Woodpecker Show",
		},
		{
			"cartoons/The New Woody Woodpecker Show (1999-2000)/1 Season/" +
				"S1ep04 Woody's Ship of Ghouls - Bad Hair Day - Downsized Woody.mkv",
			1, 4, "The New Woody Woodpecker Show",
		},
		// NxNN / Cyrillic х
		{
			"series/Melrose Place_[torrents.ru]/season 3/" +
				"Melrose Place 3х13 - Just Say No.avi",
			3, 13, "Melrose Place",
		},
		{"tv/Street Customs [Season 2]/2x05 Zach & Cody XMas Cars.ts", 2, 5, "Street Customs"},
		// Russian Сезон / Серия — full spin-off title from folder tree
		{
			"series/Универ. Новая общага/Универ. Новая общага. Сезон 06 (2014)/" +
				"Универ. Новая общага. Сезон 06. Серия 120.avi",
			6, 120, "Универ. Новая общага",
		},
		{
			"series/Универ. Новая общага/Универ. Новая общага. Сезон 01 (2011)/" +
				"Универ. Новая общага. Сезон 01. Серия 008.avi",
			1, 8, "Универ. Новая общага",
		},
		{
			"series/Интерны/Интерны. Сезон №11. Серии №205-224/" +
				"Интерны. Сезон №11. Серия №219.avi",
			11, 219, interny,
		},
		{
			"series/Интерны/Интерны. Сезон №1. Серии №001-020/" +
				"Интерны. Сезон №1. Серия №006.avi",
			1, 6, interny,
		},
		// (N.seriya) / (N.iz.M)
		{
			"series/SashaTanya.2013.SATRip/SashaTanya.(18.seriya).2013.SATRip.avi",
			1, 18, "SashaTanya",
		},
		{
			"tv/Kak.deistvyut.narkotiki.(3.serii.iz.3).2011.x264.HDTVRip.720p/" +
				"Kak.deistvyut.narkotiki.(2.iz.3).Ekstezi.2011.x264.HDTVRip.720p.mkv",
			1, 2, "Kak deistvyut narkotiki",
		},
		// SEE packing under season folder: 208 → S02E08
		{"tv/Pimp my Ride UK [Season 2]/208.avi", 2, 8, pimpUK},
		{"tv/Pimp my Ride UK [Season 3]/304.avi", 3, 4, pimpUK},
		{
			"tv/Pimp my Ride International [Season 1]/106.avi",
			1, 6, "Pimp my Ride International",
		},
		{"tv/Pimp my Ride France Season 1/106.avi", 1, 6, "Pimp my Ride France"},
		// bare eN / showNN — parent folder is the show (not the collection root)
		{
			"cartoons/Tom and Jerry (The exotic сollection)/Том и Джерри Комедийное Шоу/e7.avi",
			1, 7, "Том и Джерри Комедийное Шоу",
		},
		{
			"cartoons/Tom and Jerry (The exotic сollection)/Новое шоу Тома и Джерри/show07.avi",
			1, 7, "Новое шоу Тома и Джерри",
		},
		// absolute / leading / mid / paren
		{
			"series/top-of-the-heap-1991-sitcom/101 The Last Temptation Of Charlie.mp4",
			1, 101, "top of the heap",
		},
		{
			"cartoons/Krot.(63 serij.iz.63).1957-2002.XviD.DVDRip/" +
				"55_Krtek_a_klobouk_(1998).avi",
			1, 55, "Krot",
		},
		{
			"cartoons/Walter Lantz/Chilly Willy/" +
				"Chilly Willy - 018 - St Mortz Blitz [DVDRip-Rus].avi",
			1, 18, "Chilly Willy",
		},
		{
			"cartoons/Tom and Jerry (The exotic сollection)/Том и Джерри. Cartoon/" +
				"071 - Кот в круизе.avi",
			1, 71, "Том и Джерри. Cartoon",
		},
		{"series/Univer/Univer_021_SATRip_by_N!CK.avi", 1, 21, "Univer"},
		{
			"tv/post.kvn.comedy.club.kvnforall/post.kvn.comedy.club.(43).kvnforall.avi",
			1, 43, "post kvn comedy club kvnforall",
		},
		{"tv/Comedy.club/Comedy.club.370.avi", 1, 370, "Comedy club"},
		{"series/QAF Reunion 2007/QAF Reunion 2007_03.avi", 1, 3, "QAF Reunion"},
		{"series/Интерны/01. Интерны. История болезни (2012).avi", 1, 1, interny},
		{
			"tv/Индустрия наркотиков/" +
				"02. Индустрия наркотиков. Марихуана_2010_HDTV 1080i.ts",
			1, 2, "Индустрия наркотиков",
		},
		{
			"cartoons/My.Dress.Up.Darling.WEBRip.1080p/" +
				"[SubsPlease] Sono Bisque Doll wa Koi wo Suru - 01 [1080p].mkv",
			1, 1, "My Dress Up Darling",
		},
		{
			"tv/Внутри мозга Билла (Inside Bill's Brain)/Part 2 [1080p].mkv",
			1, 2, "Внутри мозга Билла (Inside Bill's Brain)",
		},
		{"tv/дом 2(с 101-147)/106.avi", 1, 106, "дом 2(с 101 147)"},
		{"tv/Дом 2 как всё начиналось/44.wmv", 1, 44, "Дом 2 как всё начиналось"},
	}
}

func TestNormalizeShowKey(t *testing.T) {
	t.Parallel()

	allure.Test(t, "normalizes punctuation for grouping", func(a *allure.Context) {
		t := a.T()
		if NormalizeShowKey("Sex and the City") != NormalizeShowKey("Sex.and.the.City") {
			t.Fatal("expected matching keys")
		}
	})
}
