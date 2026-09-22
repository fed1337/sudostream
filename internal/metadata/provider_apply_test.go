package metadata

import (
	"os"
	"path/filepath"
	"sudoStream/internal/mediafs"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestProviderProvenance_Helpers(t *testing.T) {
	t.Parallel()

	allure.Test(t, "provider provenance formats and user-edit detection", func(a *allure.Context) {
		t := a.T()

		if got := FormatProviderOverriddenBy("anilist"); got != "provider:anilist" {
			t.Fatalf("provider provenance: %q", got)
		}

		userActor := FormatUserOverriddenBy("42")
		providerActor := FormatProviderOverriddenBy("anilist")

		if !IsUserOverridden(&userActor) {
			t.Fatal("want user actor detected")
		}
		if IsUserOverridden(&providerActor) || IsUserOverridden(nil) {
			t.Fatal("provider and nil actors are not user edits")
		}
	})
}

func TestProviderPatchFields_WhitelistsAndTrims(t *testing.T) {
	t.Parallel()

	allure.Test(t, "provider fields map to editable patch keys only", func(a *allure.Context) {
		t := a.T()

		blank := ""
		title := "  Steins;Gate  "
		year := 2011
		patch := providerPatchFields(ProviderFields{
			Title:       &title,
			Show:        &blank,
			Description: nil,
			Genres:      []string{"Sci-Fi", "  ", "Thriller"},
			Year:        &year,
		})

		if len(patch) != 3 {
			t.Fatalf("want title, genres, year only, got %v", patch)
		}
		if patch[fieldTitle].Value() != "Steins;Gate" {
			t.Fatalf("want trimmed title, got %v", patch[fieldTitle].Value())
		}
		if _, ok := patch[fieldShow]; ok {
			t.Fatal("blank values must not be written")
		}
		genres, ok := patch[fieldGenres].Value().([]any)
		if !ok || len(genres) != 2 {
			t.Fatalf("want blank genres dropped, got %v", patch[fieldGenres].Value())
		}
		if patch[fieldYear].Value() != float64(2011) {
			t.Fatalf("year: %v", patch[fieldYear].Value())
		}

		if len(providerPatchFields(ProviderFields{})) != 0 {
			t.Fatal("empty provider fields produce no patch")
		}
	})
}

func TestListVideoPaths_WalksLibraryRoots(t *testing.T) {
	t.Parallel()

	allure.Test(t, "video walk skips hidden dirs and non-video files", func(a *allure.Context) {
		t := a.T()
		root := t.TempDir()

		for _, rel := range []string{
			"movies/a.mkv",
			"movies/notes.txt",
			"movies/.hidden/b.mkv",
			"movies/sub/c.mp4",
			"other/d.mkv",
		} {
			absPath := filepath.Join(root, rel)
			err := os.MkdirAll(filepath.Dir(absPath), 0o750)
			if err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			err = os.WriteFile(absPath, []byte("x"), 0o600)
			if err != nil {
				t.Fatalf("write: %v", err)
			}
		}

		media, err := mediafs.New(root)
		if err != nil {
			t.Fatalf("mediafs: %v", err)
		}

		paths, err := ListVideoPaths(media, []string{"movies"})
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(paths) != 2 {
			t.Fatalf("want 2 videos under movies, got %v", paths)
		}

		_, err = ListVideoPaths(media, []string{"missing"})
		if err == nil {
			t.Fatal("want error for unknown root")
		}

		got, listErr := ListVideoPaths(nil, []string{"movies"})
		if got != nil || listErr != nil {
			t.Fatalf("nil media service: %v %v", got, listErr)
		}
	})
}
