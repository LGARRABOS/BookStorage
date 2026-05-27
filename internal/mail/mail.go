package mail

import (
	"context"

	"bookstorage/internal/config"
)

// Message is a single outbound email.
type Message struct {
	To, Subject, TextBody, HTMLBody string
}

// Sender delivers transactional email.
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

// NewSender returns a Mailjet client when configured, otherwise a no-op sender.
func NewSender(s *config.Settings) Sender {
	if s == nil || !s.MailConfigured() {
		return noopSender{}
	}
	return &mailjetClient{
		publicKey:  s.MailjetAPIKeyPublic,
		privateKey: s.MailjetAPIKeyPrivate,
		from:       s.MailFrom,
	}
}
