package email_test

import (
	"context"
	"os"
	"sudoStream/internal/email"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestNewSenderFromEnv_SelectsImplementation(t *testing.T) {
	t.Parallel()

	allure.Test(
		t,
		"empty host uses log sender, configured host uses SMTP",
		func(a *allure.Context) {
			t := a.T()
			logSender := email.NewSenderFromEnv("", "587", "", "", "from@example.com", "")
			if _, ok := logSender.(email.LogSender); !ok {
				t.Fatalf("expected LogSender for empty host, got %T", logSender)
			}

			smtpSender := email.NewSenderFromEnv(
				"smtp.example.com",
				"587",
				"user",
				"pass",
				"from@x",
				"sudoStream",
			)
			if _, ok := smtpSender.(*email.SMTPSender); !ok {
				t.Fatalf("expected SMTPSender for configured host, got %T", smtpSender)
			}
		},
	)
}

func TestLogSender_SendReturnsNil(t *testing.T) {
	t.Parallel()

	allure.Test(t, "log sender never errors", func(a *allure.Context) {
		t := a.T()
		err := email.LogSender{}.Send(context.Background(), "to@example.com", "subject", "body")
		if err != nil {
			t.Fatalf("log sender send: %v", err)
		}
	})
}

func TestSMTPSender_SendRejectsMissingHost(t *testing.T) {
	t.Parallel()

	allure.Test(t, "send fails when SMTP host is not configured", func(a *allure.Context) {
		t := a.T()
		sender := email.NewSMTPSender(
			email.SMTPConfig{
				Port: "587",
				From: "from@example.com",
			},
		)

		err := sender.Send(context.Background(), "to@example.com", "subject", "body")
		if err == nil {
			t.Fatal("expected error for missing SMTP host")
		}
	})
}

func TestSMTPSender_SendReportsDialError(t *testing.T) {
	t.Parallel()

	allure.Test(t, "send returns error when SMTP server is unreachable", func(a *allure.Context) {
		t := a.T()
		sender := email.NewSMTPSender(email.SMTPConfig{
			Host:     "127.0.0.1",
			Port:     "1", // unused reserved port; dial fails fast
			User:     "user",
			Password: "pass",
			From:     "",
			FromName: "",
		})

		err := sender.Send(context.Background(), "to@example.com", "subject", "body")
		if err == nil {
			t.Fatal("expected dial error for unreachable SMTP server")
		}
	})
}

func TestSMTPSender_SendWithoutAuth(t *testing.T) {
	t.Parallel()

	allure.Test(t, "sends mail without AUTH when SMTP user is empty", func(a *allure.Context) {
		t := a.T()
		host := os.Getenv("SUDOSTREAM_SMTP_HOST")
		if host == "" {
			host = "127.0.0.1"
		}

		port := os.Getenv("SUDOSTREAM_SMTP_PORT")
		if port == "" {
			port = "1025"
		}

		sender := email.NewSMTPSender(
			email.SMTPConfig{
				Host: host,
				Port: port,
				From: "sudostream@localhost",
			},
		)

		err := sender.Send(context.Background(), "user@example.com", "invite test", "hello")
		if err != nil {
			t.Skipf("smtp not reachable at %s:%s: %v", host, port, err)
		}
	})
}
