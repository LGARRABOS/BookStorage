package mail

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const defaultMailjetSendURL = "https://api.mailjet.com/v3.1/send"

// mailSendHook is set by tests to stub outbound delivery.
var mailSendHook func(ctx context.Context, msg Message) error

// SetSendHook replaces outbound delivery for tests. Pass nil to restore default behavior.
func SetSendHook(h func(context.Context, Message) error) {
	mailSendHook = h
}

type mailjetClient struct {
	publicKey  string
	privateKey string
	from       string
	sendURL    string
	httpClient *http.Client
}

type mailjetAddress struct {
	Email string `json:"Email"`
	Name  string `json:"Name,omitempty"`
}

type mailjetMessage struct {
	From     mailjetAddress   `json:"From"`
	To       []mailjetAddress `json:"To"`
	Subject  string           `json:"Subject"`
	TextPart string           `json:"TextPart,omitempty"`
	HTMLPart string           `json:"HTMLPart,omitempty"`
}

type mailjetSendRequest struct {
	Messages []mailjetMessage `json:"Messages"`
}

func (c *mailjetClient) Send(ctx context.Context, msg Message) error {
	if mailSendHook != nil {
		return mailSendHook(ctx, msg)
	}
	if strings.TrimSpace(msg.To) == "" {
		return fmt.Errorf("mail: empty recipient")
	}
	fromName, fromEmail := parseFromAddress(c.from)
	if fromEmail == "" {
		return fmt.Errorf("mail: invalid From address")
	}
	payload := mailjetSendRequest{
		Messages: []mailjetMessage{{
			From:     mailjetAddress{Email: fromEmail, Name: fromName},
			To:       []mailjetAddress{{Email: strings.TrimSpace(msg.To)}},
			Subject:  msg.Subject,
			TextPart: msg.TextBody,
			HTMLPart: msg.HTMLBody,
		}},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	sendURL := c.sendURL
	if sendURL == "" {
		sendURL = defaultMailjetSendURL
	}
	client := c.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sendURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	auth := base64.StdEncoding.EncodeToString([]byte(c.publicKey + ":" + c.privateKey))
	req.Header.Set("Authorization", "Basic "+auth)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("mailjet send: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
}

func parseFromAddress(raw string) (name, email string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	if i := strings.LastIndex(raw, "<"); i >= 0 {
		if j := strings.LastIndex(raw, ">"); j > i {
			name = strings.TrimSpace(raw[:i])
			name = strings.Trim(name, `"`)
			email = strings.TrimSpace(raw[i+1 : j])
			return name, email
		}
	}
	return "", raw
}

func newMailjetClientForTest(public, private, from, sendURL string, client *http.Client) *mailjetClient {
	return &mailjetClient{
		publicKey:  public,
		privateKey: private,
		from:       from,
		sendURL:    sendURL,
		httpClient: client,
	}
}
