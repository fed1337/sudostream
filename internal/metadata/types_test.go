package metadata

import (
	"encoding/json"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestJSONValue_UnmarshalNullAndScalar(t *testing.T) {
	t.Parallel()

	allure.Test(t, "jsonValue decodes null and string patch values", func(a *allure.Context) {
		t := a.T()
		var nullValue jsonValue
		err := json.Unmarshal([]byte(`null`), &nullValue)
		if err != nil {
			t.Fatalf("unmarshal null: %v", err)
		}
		if !nullValue.IsNull() {
			t.Fatal("expected null patch value")
		}

		var stringValue jsonValue
		err = json.Unmarshal([]byte(`"Pilot"`), &stringValue)
		if err != nil {
			t.Fatalf("unmarshal string: %v", err)
		}
		if stringValue.Value() != testEpisodeName {
			t.Fatalf("unexpected string value: %#v", stringValue.Value())
		}
	})
}

func TestJSONValue_UnmarshalList(t *testing.T) {
	t.Parallel()

	allure.Test(t, "jsonValue decodes string list patch values", func(a *allure.Context) {
		t := a.T()
		var listValue jsonValue
		err := json.Unmarshal([]byte(`["drama","sci-fi"]`), &listValue)
		if err != nil {
			t.Fatalf("unmarshal list: %v", err)
		}

		raw := listValue.Value()
		items, ok := raw.([]any)
		if !ok || len(items) != 2 {
			t.Fatalf("unexpected list value: %#v", raw)
		}
	})
}

func TestPatchRequest_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	allure.Test(t, "patch request unmarshals target and fields", func(a *allure.Context) {
		t := a.T()
		var request PatchRequest
		err := json.Unmarshal([]byte(`{
			"target":"override",
			"fields":{"title":"Pilot","season":1,"genres":["drama"]}
		}`), &request)
		if err != nil {
			t.Fatalf("unmarshal patch request: %v", err)
		}
		if request.Target != PatchTargetOverride {
			t.Fatalf("unexpected target: %q", request.Target)
		}
		if request.Fields["title"] == nil || request.Fields["title"].Value() != "Pilot" {
			t.Fatalf("unexpected title field: %#v", request.Fields["title"])
		}
	})
}

func TestPatchRequest_JSONNullClearsField(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"JSON null field becomes nil map entry that clears on apply",
		func(a *allure.Context) {
			t := a.T()
			var request PatchRequest
			err := json.Unmarshal([]byte(`{"target":"override","fields":{"title":null}}`), &request)
			if err != nil {
				t.Fatalf("unmarshal null title: %v", err)
			}
			if _, ok := request.Fields["title"]; !ok {
				t.Fatal("expected title key present for JSON null")
			}
			if request.Fields["title"] != nil {
				t.Fatalf("expected nil *jsonValue for JSON null, got %#v", request.Fields["title"])
			}

			keep := testKeepTitle
			updated, err := ApplyPatchFields(
				StoredOverride{VideoFields: VideoFields{Title: &keep}},
				request.Fields,
			)
			if err != nil {
				t.Fatalf("apply null patch: %v", err)
			}
			if updated.Title != nil {
				t.Fatalf("expected cleared title, got %#v", updated.Title)
			}
		},
	)
}
