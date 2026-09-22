package metadata

import (
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

const (
	testFilmTitle     = "Inception"
	testShowName      = "Demo Show"
	testEpisodeName   = "Pilot"
	testOverrideTitle = "Override"
	testKeepTitle     = "Keep"
)

const sampleFFProbeJSON = `{
  "format": {
    "duration": "3723.456000",
    "bit_rate": "4500000",
    "tags": {
      "title": "Pilot",
      "show": "Demo Show",
      "season_number": "1",
      "episode_id": "2",
      "genre": "Drama,Sci-Fi"
    }
  },
  "streams": [
    {
      "codec_type": "video",
      "codec_name": "h264",
      "width": 1920,
      "height": 1080,
      "r_frame_rate": "24000/1001"
    },
    {
      "codec_type": "audio",
      "codec_name": "aac",
      "channels": 2,
      "tags": { "language": "eng" }
    }
  ]
}`

func TestParseFFProbeJSON_MapsTagsAndStreams(t *testing.T) {
	t.Parallel()

	allure.Test(t, "ffprobe json maps tags and stream fields", func(a *allure.Context) {
		t := a.T()
		fields, err := ParseFFProbeJSON([]byte(sampleFFProbeJSON))
		if err != nil {
			t.Fatalf("parse ffprobe json: %v", err)
		}

		assertStringField(t, "title", fields.Title, testEpisodeName)
		assertStringField(t, "show", fields.Show, testShowName)
		assertIntField(t, "season", fields.Season, 1)
		assertIntField(t, "episode", fields.Episode, 2)
		if len(fields.Genres) != 2 {
			t.Fatalf("expected 2 genres, got %#v", fields.Genres)
		}
		assertStringField(t, "video codec", fields.VideoCodec, "h264")
		assertStringField(t, "audio codec", fields.AudioCodec, "aac")
	})
}

func TestParseFFProbeJSON_IgnoresStreamTitles(t *testing.T) {
	t.Parallel()

	allure.Test(t, "audio stream title is not used as file title", func(a *allure.Context) {
		t := a.T()
		raw := []byte(`{
  "format": {"duration": "10.0", "bit_rate": "1000", "tags": {}},
  "streams": [
    {"codec_type": "video", "codec_name": "h264", "width": 1280, "height": 720, "r_frame_rate": "25/1"},
    {"codec_type": "audio", "codec_name": "ac3", "channels": 2,
     "tags": {"language": "rus", "title": "DD 2.0 @ 192 kbps (MVO NTV)"}}
  ]
}`)
		fields, err := ParseFFProbeJSON(raw)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if fields.Title != nil {
			t.Fatalf("expected no title from stream tag, got %#v", fields.Title)
		}
		if len(fields.AudioLanguages) != 1 || fields.AudioLanguages[0] != "rus" {
			t.Fatalf("expected rus audio language, got %#v", fields.AudioLanguages)
		}
	})
}

func assertStringField(t *testing.T, name string, value *string, want string) {
	t.Helper()
	if value == nil || *value != want {
		t.Fatalf("unexpected %s: %#v", name, value)
	}
}

func assertIntField(t *testing.T, name string, value *int, want int) {
	t.Helper()
	if value == nil || *value != want {
		t.Fatalf("unexpected %s: %#v", name, value)
	}
}
