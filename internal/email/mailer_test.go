package email_test

import (
	"context"
	"strings"
	"sudoStream/internal/email"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

type captureSender struct {
	last email.Message
}

func (s *captureSender) Send(_ context.Context, msg email.Message) error {
	s.last = msg

	return nil
}

func TestMailer_AuthTemplatesIncludeCTALink(t *testing.T) {
	t.Parallel()

	const (
		baseURL = "http://localhost:8080"
		link    = "http://localhost:8080/accept-invite?token=abc123"
		ttl     = 24 * time.Hour
	)

	cases := []struct {
		name    string
		subject string
		send    func(*email.Mailer, context.Context) error
		needle  string
	}{
		{
			name:    "invite",
			subject: "You're invited to sudoStream",
			needle:  link,
			send: func(mailer *email.Mailer, ctx context.Context) error {
				return mailer.SendInvite(ctx, "user@example.com", link, ttl)
			},
		},
		{
			name:    "confirm",
			subject: "Confirm your sudoStream email",
			needle:  "http://localhost:8080/confirm-email?token=abc123",
			send: func(mailer *email.Mailer, ctx context.Context) error {
				return mailer.SendConfirmEmail(
					ctx,
					"user@example.com",
					"http://localhost:8080/confirm-email?token=abc123",
					ttl,
				)
			},
		},
		{
			name:    "change-email",
			subject: "Confirm your new sudoStream email",
			needle:  "http://localhost:8080/confirm-email?token=xyz",
			send: func(mailer *email.Mailer, ctx context.Context) error {
				return mailer.SendChangeEmail(
					ctx,
					"new@example.com",
					"http://localhost:8080/confirm-email?token=xyz",
					ttl,
				)
			},
		},
		{
			name:    "reset",
			subject: "Reset your sudoStream password",
			needle:  "http://localhost:8080/reset-password?token=reset1",
			send: func(mailer *email.Mailer, ctx context.Context) error {
				return mailer.SendResetPassword(
					ctx,
					"user@example.com",
					"http://localhost:8080/reset-password?token=reset1",
					ttl,
				)
			},
		},
	}

	for _, tc := range cases {
		allure.Test(t, "mailer renders HTML+plain CTA — "+tc.name, func(a *allure.Context) {
			t := a.T()
			capture := &captureSender{}
			mailer := email.NewMailer(capture, baseURL)
			err := tc.send(mailer, context.Background())
			if err != nil {
				t.Fatalf("send: %v", err)
			}
			if capture.last.Subject != tc.subject {
				t.Fatalf("subject=%q want %q", capture.last.Subject, tc.subject)
			}
			if !strings.Contains(capture.last.Text, tc.needle) {
				t.Fatalf("plain missing link %q in %q", tc.needle, capture.last.Text)
			}
			if !strings.Contains(capture.last.HTML, tc.needle) {
				t.Fatalf("html missing link %q", tc.needle)
			}
			if !strings.Contains(capture.last.HTML, "<") {
				t.Fatal("expected HTML markup")
			}
		})
	}
}
