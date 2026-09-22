package metadata

import (
	"strings"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestBuildSearchDocument_SceneReleaseIncludesShow(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"search document includes friends from a scene-release filename",
		func(a *allure.Context) {
			t := a.T()
			relPath := "friends/[linuxisos.ru].friends.s01.e01.mkv"
			document := BuildSearchDocument(relPath, VideoFields{}, VideoFields{})
			if !strings.Contains(document, "friends") {
				t.Fatalf("document missing friends: %q", document)
			}

			tokens := NormalizeSearchQuery("Friends")
			if len(tokens) != 1 || tokens[0] != "friends" {
				t.Fatalf("tokens=%v", tokens)
			}
		},
	)
}

func TestBuildSearchDocument_IncludesOverrideTitle(t *testing.T) {
	t.Parallel()

	allure.Test(t, "override title is copied into the search document", func(a *allure.Context) {
		t := a.T()
		title := "Central Perk"
		document := BuildSearchDocument("video.mkv", VideoFields{}, VideoFields{Title: &title})
		if !strings.Contains(document, "central perk") {
			t.Fatalf("document missing override title: %q", document)
		}
	})
}
