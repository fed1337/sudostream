package email

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/matcornic/hermes/v2"
)

const (
	productName      = "sudoStream"
	actionColor      = "#2563eb"
	resetColor       = "#dc2626"
	defaultBaseURL   = "http://localhost:8080"
	defaultSignature = "Thanks"
)

// Mailer renders branded transactional emails and delivers them via Sender.
type Mailer struct {
	sender Sender
	hermes hermes.Hermes
}

// NewMailer constructs a Mailer. baseURL is the public app origin (no trailing slash).
func NewMailer(sender Sender, baseURL string) *Mailer {
	if sender == nil {
		sender = LogSender{}
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	return &Mailer{
		sender: sender,
		hermes: hermes.Hermes{
			Product: hermes.Product{
				Name:      productName,
				Link:      baseURL,
				Copyright: "© sudoStream",
			},
		},
	}
}

// SendInvite emails an accept-invite CTA.
func (m *Mailer) SendInvite(
	ctx context.Context,
	to, link string,
	expiresIn time.Duration,
) error {
	return m.sendAction(ctx, to, "You're invited to sudoStream", hermes.Email{
		Body: hermes.Body{
			Title:  "You're invited",
			Intros: []string{"An administrator invited you to sudoStream."},
			Actions: []hermes.Action{{
				Instructions: "Click the button below to accept the invite and set a password:",
				Button: hermes.Button{
					Color: actionColor,
					Text:  "Accept invite",
					Link:  link,
				},
			}},
			Outros: []string{
				fmt.Sprintf("This invite expires in %s.", formatTTL(expiresIn)),
				"If you were not expecting this invite, you can ignore this email.",
			},
			Signature: defaultSignature,
		},
	})
}

// SendConfirmEmail emails an email-confirmation CTA.
func (m *Mailer) SendConfirmEmail(
	ctx context.Context,
	to, link string,
	expiresIn time.Duration,
) error {
	return m.sendAction(ctx, to, "Confirm your sudoStream email", hermes.Email{
		Body: hermes.Body{
			Title:  "Confirm your email",
			Intros: []string{"Please confirm your email address to activate your sudoStream account."},
			Actions: []hermes.Action{{
				Instructions: "Click the button below to confirm your email:",
				Button: hermes.Button{
					Color: actionColor,
					Text:  "Confirm email",
					Link:  link,
				},
			}},
			Outros: []string{
				fmt.Sprintf("This link expires in %s.", formatTTL(expiresIn)),
				"If you did not create an account, you can ignore this email.",
			},
			Signature: defaultSignature,
		},
	})
}

// SendChangeEmail emails a change-email confirmation CTA.
func (m *Mailer) SendChangeEmail(
	ctx context.Context,
	to, link string,
	expiresIn time.Duration,
) error {
	return m.sendAction(ctx, to, "Confirm your new sudoStream email", hermes.Email{
		Body: hermes.Body{
			Title:  "Confirm your new email",
			Intros: []string{"Confirm this address to finish changing your sudoStream email."},
			Actions: []hermes.Action{{
				Instructions: "Click the button below to confirm your new email:",
				Button: hermes.Button{
					Color: actionColor,
					Text:  "Confirm new email",
					Link:  link,
				},
			}},
			Outros: []string{
				fmt.Sprintf("This link expires in %s.", formatTTL(expiresIn)),
				"If you did not request an email change, you can ignore this email.",
			},
			Signature: defaultSignature,
		},
	})
}

// SendResetPassword emails a password-reset CTA.
func (m *Mailer) SendResetPassword(
	ctx context.Context,
	to, link string,
	expiresIn time.Duration,
) error {
	return m.sendAction(ctx, to, "Reset your sudoStream password", hermes.Email{
		Body: hermes.Body{
			Title: "Reset your password",
			Intros: []string{
				"We received a request to reset the password for your sudoStream account.",
			},
			Actions: []hermes.Action{{
				Instructions: "Click the button below to choose a new password:",
				Button: hermes.Button{
					Color: resetColor,
					Text:  "Reset password",
					Link:  link,
				},
			}},
			Outros: []string{
				fmt.Sprintf("This link expires in %s.", formatTTL(expiresIn)),
				"If you did not request a password reset, no further action is required.",
			},
			Signature: defaultSignature,
		},
	})
}

func (m *Mailer) sendAction(
	ctx context.Context,
	recipient, subject string,
	template hermes.Email,
) error {
	htmlBody, err := m.hermes.GenerateHTML(template)
	if err != nil {
		return fmt.Errorf("generate html email: %w", err)
	}
	textBody, err := m.hermes.GeneratePlainText(template)
	if err != nil {
		return fmt.Errorf("generate plain email: %w", err)
	}

	err = m.sender.Send(ctx, Message{
		To:      recipient,
		Subject: subject,
		Text:    textBody,
		HTML:    htmlBody,
	})
	if err != nil {
		return fmt.Errorf("send email: %w", err)
	}

	return nil
}

func formatTTL(expiresIn time.Duration) string {
	if expiresIn <= 0 {
		return "a limited time"
	}

	return expiresIn.Round(time.Minute).String()
}
