// Package email sends outbound mail for auth flows.
package email

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/smtp"
	"strings"
)

var errSMTPNotConfigured = errors.New("smtp host is not configured")
var errInvalidEmailHeader = errors.New("email header contains invalid control characters")
var errInvalidRecipient = errors.New("recipient contains invalid control characters")

// Sender delivers email messages.
type Sender interface {
	Send(ctx context.Context, recipient, subject, body string) error
}

// LogSender prints messages to the application log (development/tests).
type LogSender struct{}

// Send logs the email instead of delivering it.
func (LogSender) Send(_ context.Context, recipient, subject, body string) error {
	log.Printf("email to=%s subject=%q\n%s", recipient, subject, body)

	return nil
}

// SMTPConfig configures SMTP delivery.
type SMTPConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	From     string
	FromName string
}

// SMTPSender delivers mail via SMTP.
type SMTPSender struct {
	config SMTPConfig
}

// NewSMTPSender constructs an SMTP sender.
func NewSMTPSender(config SMTPConfig) *SMTPSender {
	return &SMTPSender{config: config}
}

// Send delivers a plain-text email message.
func (s *SMTPSender) Send(_ context.Context, recipient, subject, body string) error {
	if strings.TrimSpace(s.config.Host) == "" {
		return fmt.Errorf("smtp host is not configured: %w", errSMTPNotConfigured)
	}

	recipient = strings.TrimSpace(recipient)
	if hasHeaderControlChars(recipient) {
		return fmt.Errorf("invalid recipient: %w", errInvalidRecipient)
	}
	if hasHeaderControlChars(subject) {
		return fmt.Errorf("invalid subject: %w", errInvalidEmailHeader)
	}
	body = sanitizePlainTextBody(body)

	fromAddr := strings.TrimSpace(s.config.From)
	if fromAddr == "" {
		fromAddr = strings.TrimSpace(s.config.User)
	}

	msg := strings.Join([]string{
		"From: " + formatFromHeader(s.config.FromName, fromAddr),
		"To: " + recipient,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
	}, "\r\n")

	addr := s.config.Host + ":" + s.config.Port
	var auth smtp.Auth
	if strings.TrimSpace(s.config.User) != "" {
		auth = smtp.PlainAuth("", s.config.User, s.config.Password, s.config.Host)
	}

	err := smtp.SendMail(addr, auth, fromAddr, []string{recipient}, []byte(msg))
	if err != nil {
		return fmt.Errorf("smtp send: %w", err)
	}

	return nil
}

func hasHeaderControlChars(value string) bool {
	return strings.ContainsAny(value, "\r\n")
}

func sanitizePlainTextBody(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")

	return body
}

func formatFromHeader(name, address string) string {
	name = strings.TrimSpace(name)
	address = strings.TrimSpace(address)
	if name == "" {
		return address
	}
	if hasHeaderControlChars(name) || hasHeaderControlChars(address) {
		return address
	}

	escaped := strings.ReplaceAll(name, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)

	return fmt.Sprintf(`"%s" <%s>`, escaped, address)
}

// NewSenderFromEnv returns SMTP sender when configured, otherwise a log sender.
//
//nolint:ireturn // callers depend on the Sender interface at the composition root.
func NewSenderFromEnv(host, port, user, password, from, fromName string) Sender {
	if strings.TrimSpace(host) == "" {
		return LogSender{}
	}

	return NewSMTPSender(SMTPConfig{
		Host:     host,
		Port:     port,
		User:     user,
		Password: password,
		From:     from,
		FromName: fromName,
	})
}
