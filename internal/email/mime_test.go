package email

import (
	"strings"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestBuildMIMEPayload_MultipartAlternative(t *testing.T) {
	t.Parallel()

	allure.Test(t, "builds multipart/alternative with plain then HTML", func(a *allure.Context) {
		t := a.T()
		payload, err := buildMIMEPayload(
			"from@example.com",
			"to@example.com",
			"Hello",
			"plain link token=abc",
			"<p>html link token=abc</p>",
		)
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		raw := string(payload)
		if !strings.Contains(raw, "Content-Type: multipart/alternative;") {
			t.Fatalf("missing multipart header: %s", raw)
		}
		if !strings.Contains(raw, "\r\n\r\n--") {
			t.Fatal("headers must end with blank CRLF line before multipart body")
		}
		if !strings.Contains(raw, "Content-Type: text/plain; charset=UTF-8") {
			t.Fatal("missing plain part")
		}
		if !strings.Contains(raw, "Content-Type: text/html; charset=UTF-8") {
			t.Fatal("missing html part")
		}
		if !strings.Contains(raw, "plain link token=abc") {
			t.Fatal("missing plain body")
		}
		if !strings.Contains(raw, "<p>html link token=abc</p>") {
			t.Fatal("missing html body")
		}
	})
}

func TestBuildMIMEPayload_PlainOnly(t *testing.T) {
	t.Parallel()

	allure.Test(t, "falls back to text/plain when HTML empty", func(a *allure.Context) {
		t := a.T()
		payload, err := buildMIMEPayload(
			"from@example.com",
			"to@example.com",
			"Hello",
			"just text",
			"",
		)
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		raw := string(payload)
		if !strings.Contains(raw, "Content-Type: text/plain; charset=UTF-8") {
			t.Fatalf("want plain: %s", raw)
		}
		if strings.Contains(raw, "multipart/") {
			t.Fatal("should not be multipart without HTML")
		}
	})
}
