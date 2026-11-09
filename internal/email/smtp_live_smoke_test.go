package email_test

import (
	"context"
	"os"
	"sudoStream/internal/email"
	"testing"
	"time"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestSMTPLive_MailpitAcceptsMultipart(t *testing.T) {
	if os.Getenv("SUDOSTREAM_LIVE_SMTP") == "" {
		t.Skip("SUDOSTREAM_LIVE_SMTP not set")
	}
	allure.Test(t, "mailpit accepts hermes multipart messages", func(a *allure.Context) {
		t := a.T()
		host := os.Getenv("SUDOSTREAM_SMTP_HOST")
		if host == "" {
			host = "127.0.0.1"
		}
		port := os.Getenv("SUDOSTREAM_SMTP_PORT")
		if port == "" {
			port = "1025"
		}
		mailer := email.NewMailer(email.NewSMTPSender(email.SMTPConfig{
			Host:     host,
			Port:     port,
			From:     "sudostream@router.lan",
			FromName: "sudoStream",
		}), "http://localhost:8080")
		err := mailer.SendResetPassword(
			context.Background(),
			"auto@hpserver.lan",
			"http://localhost:8080/reset-password?token=smoke123",
			24*time.Hour,
		)
		if err != nil {
			t.Fatalf("reset: %v", err)
		}
		err = mailer.SendInvite(
			context.Background(),
			"invite-e47@hpserver.lan",
			"http://localhost:8080/accept-invite?token=invite123",
			24*time.Hour,
		)
		if err != nil {
			t.Fatalf("invite: %v", err)
		}
	})
}
