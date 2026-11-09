// Package email sends outbound mail for auth flows.
package email

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"mime/multipart"
	"net/smtp"
	"net/textproto"
	"strings"
)

var (
	errSMTPNotConfigured  = errors.New("smtp host is not configured")
	errInvalidEmailHeader = errors.New("email header contains invalid control characters")
	errInvalidRecipient   = errors.New("recipient contains invalid control characters")
	errEmptyMessageBody   = errors.New("email message has empty body")
)

// Message is an outbound email with plain and optional HTML bodies.
type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
}

// Sender delivers email messages.
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

// LogSender prints messages to the application log (development/tests).
type LogSender struct{}

// Send logs the email instead of delivering it.
func (LogSender) Send(_ context.Context, msg Message) error {
	log.Printf(
		"email to=%s subject=%q html_bytes=%d\n%s",
		msg.To,
		msg.Subject,
		len(msg.HTML),
		msg.Text,
	)

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

// Send delivers a multipart (or plain) email message.
func (s *SMTPSender) Send(_ context.Context, msg Message) error {
	if strings.TrimSpace(s.config.Host) == "" {
		return fmt.Errorf("smtp host is not configured: %w", errSMTPNotConfigured)
	}

	recipient := strings.TrimSpace(msg.To)
	if hasHeaderControlChars(recipient) {
		return fmt.Errorf("invalid recipient: %w", errInvalidRecipient)
	}
	if hasHeaderControlChars(msg.Subject) {
		return fmt.Errorf("invalid subject: %w", errInvalidEmailHeader)
	}

	text := msg.Text
	htmlBody := msg.HTML
	if strings.TrimSpace(text) == "" && strings.TrimSpace(htmlBody) == "" {
		return fmt.Errorf("empty email body: %w", errEmptyMessageBody)
	}

	fromAddr := strings.TrimSpace(s.config.From)
	if fromAddr == "" {
		fromAddr = strings.TrimSpace(s.config.User)
	}

	payload, err := buildMIMEPayload(
		formatFromHeader(s.config.FromName, fromAddr),
		recipient,
		msg.Subject,
		text,
		htmlBody,
	)
	if err != nil {
		return fmt.Errorf("build mime payload: %w", err)
	}

	addr := s.config.Host + ":" + s.config.Port
	var auth smtp.Auth
	if strings.TrimSpace(s.config.User) != "" {
		auth = smtp.PlainAuth("", s.config.User, s.config.Password, s.config.Host)
	}

	err = smtp.SendMail(addr, auth, fromAddr, []string{recipient}, payload)
	if err != nil {
		return fmt.Errorf("smtp send: %w", err)
	}

	return nil
}

func buildMIMEPayload(from, recipient, subject, text, htmlBody string) ([]byte, error) {
	text = ensureCRLF(text)
	htmlBody = ensureCRLF(htmlBody)

	if strings.TrimSpace(htmlBody) == "" {
		return []byte(strings.Join([]string{
			"From: " + from,
			"To: " + recipient,
			"Subject: " + subject,
			"MIME-Version: 1.0",
			"Content-Type: text/plain; charset=UTF-8",
			"",
			text,
		}, "\r\n")), nil
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	plainHeader := make(textproto.MIMEHeader)
	plainHeader.Set("Content-Type", "text/plain; charset=UTF-8")
	plainPart, err := writer.CreatePart(plainHeader)
	if err != nil {
		return nil, fmt.Errorf("create plain part: %w", err)
	}
	_, err = plainPart.Write([]byte(text))
	if err != nil {
		return nil, fmt.Errorf("write plain part: %w", err)
	}

	htmlHeader := make(textproto.MIMEHeader)
	htmlHeader.Set("Content-Type", "text/html; charset=UTF-8")
	htmlPart, err := writer.CreatePart(htmlHeader)
	if err != nil {
		return nil, fmt.Errorf("create html part: %w", err)
	}
	_, err = htmlPart.Write([]byte(htmlBody))
	if err != nil {
		return nil, fmt.Errorf("write html part: %w", err)
	}

	err = writer.Close()
	if err != nil {
		return nil, fmt.Errorf("close multipart writer: %w", err)
	}

	// Header block must end with a blank CRLF line before the multipart body.
	// strings.Join(..., "",) does NOT create that blank line.
	headers := strings.Join([]string{
		"From: " + from,
		"To: " + recipient,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		`Content-Type: multipart/alternative; boundary="` + writer.Boundary() + `"`,
	}, "\r\n") + "\r\n\r\n"

	return append([]byte(headers), body.Bytes()...), nil
}

func hasHeaderControlChars(value string) bool {
	return strings.ContainsAny(value, "\r\n")
}

func ensureCRLF(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")

	return strings.ReplaceAll(body, "\n", "\r\n")
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
