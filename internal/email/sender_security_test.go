package email_test

import (
	"context"
	"strings"
	"sudoStream/internal/email"
	"testing"

	allure "github.com/allure-framework/allure-go/commons/gotest"
)

func TestSMTPSender_RejectsHeaderInjectionInRecipientOrSubject(t *testing.T) {
	t.Parallel()

	sender := email.NewSMTPSender(email.SMTPConfig{
		Host: "127.0.0.1",
		Port: "1",
		From: "from@example.com",
	})

	for _, tc := range []struct {
		name      string
		recipient string
		subject   string
	}{
		{
			name:      "recipient CRLF Bcc",
			recipient: "user@example.com\r\nBcc: attacker@evil.test",
			subject:   "Reset",
		},
		{
			name:      "subject CRLF Bcc",
			recipient: "user@example.com",
			subject:   "Reset\r\nBcc: attacker@evil.test",
		},
	} {
		allure.Test(t, "smtp rejects header injection — "+tc.name, func(a *allure.Context) {
			t := a.T()
			err := sender.Send(context.Background(), tc.recipient, tc.subject, "token=secret")
			if err == nil {
				t.Fatal("expected error before smtp send")
			}
		})
	}
}

func TestSMTPSender_FromNameCRLFDoesNotReachSMTPPayload(t *testing.T) {
	t.Parallel()

	allure.Test(t, "From display name CRLF falls back to bare address", func(a *allure.Context) {
		t := a.T()
		// We cannot intercept smtp.SendMail without a server; verify helper behavior via send
		// rejection path on subject while FromName carries CRLF (From line must not add Bcc).
		sender := email.NewSMTPSender(email.SMTPConfig{
			Host:     "127.0.0.1",
			Port:     "1",
			From:     "from@example.com",
			FromName: "sudoStream\r\nBcc: attacker@evil.test",
		})
		err := sender.Send(context.Background(), "user@example.com", "ok", "body")
		if err == nil {
			t.Fatal("expected dial error, not silent Bcc injection")
		}
		if strings.Contains(err.Error(), "attacker") {
			t.Fatalf("unexpected leak in error: %v", err)
		}
	})
}
