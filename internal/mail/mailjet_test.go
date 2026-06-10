package mail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMailjetClientSend(t *testing.T) {
	var gotAuth string
	var got mailjetSendRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"Messages":[{"Status":"success"}]}`))
	}))
	defer srv.Close()

	client := newMailjetClientForTest("pub-key", "priv-key", "BookStorage <noreply@example.com>", srv.URL, srv.Client())
	err := client.Send(context.Background(), Message{
		To:       "user@example.com",
		Subject:  "Test",
		TextBody: "Hello",
		HTMLBody: "<p>Hello</p>",
	})
	if err != nil {
		t.Fatal(err)
	}
	expectedAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("pub-key:priv-key"))
	if gotAuth != expectedAuth {
		t.Fatalf("auth %q want %q", gotAuth, expectedAuth)
	}
	if len(got.Messages) != 1 {
		t.Fatalf("messages: %+v", got.Messages)
	}
	msg := got.Messages[0]
	if msg.From.Email != "noreply@example.com" || msg.From.Name != "BookStorage" {
		t.Fatalf("from: %+v", msg.From)
	}
	if len(msg.To) != 1 || msg.To[0].Email != "user@example.com" {
		t.Fatalf("to: %+v", msg.To)
	}
	if msg.Subject != "Test" || msg.TextPart != "Hello" || msg.HTMLPart != "<p>Hello</p>" {
		t.Fatalf("payload: %+v", msg)
	}
}

func TestMailjetClientSendErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"ErrorMessage":"bad auth"}`)
	}))
	defer srv.Close()

	client := newMailjetClientForTest("a", "b", "noreply@example.com", srv.URL, srv.Client())
	err := client.Send(context.Background(), Message{To: "x@y.z", Subject: "s"})
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected 401 error, got %v", err)
	}
}

func TestParseFromAddress(t *testing.T) {
	name, email := parseFromAddress(`BookStorage <noreply@test.com>`)
	if name != "BookStorage" || email != "noreply@test.com" {
		t.Fatalf("got %q %q", name, email)
	}
	name, email = parseFromAddress("plain@example.com")
	if name != "" || email != "plain@example.com" {
		t.Fatalf("got %q %q", name, email)
	}
}

func TestNoopSender(t *testing.T) {
	if err := (noopSender{}).Send(context.Background(), Message{To: "a@b.c"}); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultMailjetHTTPClientHasTimeout(t *testing.T) {
	if defaultMailjetHTTPClient.Timeout != defaultMailjetHTTPTimeout {
		t.Fatalf("timeout %v, want %v", defaultMailjetHTTPClient.Timeout, defaultMailjetHTTPTimeout)
	}
}
