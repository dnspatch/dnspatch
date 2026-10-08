package shoutrrr

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dnspatch/dnspatch/plugin"
)

func build(t *testing.T, urls ...string) (plugin.Notifier, error) {
	t.Helper()

	list := make([]any, len(urls))
	for i, u := range urls {
		list[i] = u
	}

	return plugin.Default.BuildNotifier(Name, map[string]any{"urls": list})
}

// ntfyURL points the ntfy service at a test server over plain HTTP.
func ntfyURL(srv *httptest.Server, topic string) string {
	return "ntfy://" + strings.TrimPrefix(srv.URL, "http://") + "/" + topic + "?scheme=http"
}

func TestNotifierRequiresURLs(t *testing.T) {
	_, err := plugin.Default.BuildNotifier(Name, map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "urls") {
		t.Errorf("BuildNotifier(no urls) error = %v, want it to name the missing urls", err)
	}
}

func TestNotifierRejectsAnUnknownServiceAtStartup(t *testing.T) {
	_, err := build(t, "nosuchservice://token-1234567890@host/topic")
	if err == nil {
		t.Fatal("BuildNotifier succeeded on an unknown service, want an error")
	}

	if strings.Contains(err.Error(), "token-1234567890") {
		t.Errorf("error %q repeats the token of the URL", err)
	}
}

func TestPublishSendsTheEventAsText(t *testing.T) {
	var (
		mu   sync.Mutex
		body string
		head http.Header
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)

		_, _ = w.Write([]byte("{}"))

		mu.Lock()
		body, head = string(data), r.Header.Clone()
		mu.Unlock()
	}))
	t.Cleanup(srv.Close)

	n, err := build(t, ntfyURL(srv, "alerts"))
	if err != nil {
		t.Fatalf("BuildNotifier: %v", err)
	}

	payload := `{"event":"ip_change","severity":"info","instance":"home","changes":[{"provider":"cf","family":"ipv4","old":"1.1.1.1","new":"2.2.2.2"}]}`
	if err := n.Publish(context.Background(), "dnspatch.events.home", []byte(payload)); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if !strings.Contains(body, "cf ipv4: 1.1.1.1 -> 2.2.2.2") {
		t.Errorf("body = %q, want the change in words", body)
	}

	if got := head.Get("Title"); got != "dnspatch: home" {
		t.Errorf("Title = %q, want %q", got, "dnspatch: home")
	}
}

func TestPublishErrorHidesTheSecretsOfTheURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	n, err := build(t, ntfyURL(srv, "supersecrettopic"))
	if err != nil {
		t.Fatalf("BuildNotifier: %v", err)
	}

	err = n.Publish(context.Background(), "x", []byte(`{"event":"cycle","success":false}`))
	if err == nil {
		t.Fatal("Publish to a failing server succeeded, want an error")
	}

	if strings.Contains(err.Error(), "supersecrettopic") {
		t.Errorf("error %q repeats the secret topic", err)
	}
}

func TestPublishHonoursTheContextWhileTheServerIsSilent(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))

	t.Cleanup(func() {
		close(release)
		srv.Close()
	})

	n, err := build(t, ntfyURL(srv, "alerts"))
	if err != nil {
		t.Fatalf("BuildNotifier: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()

	if err := n.Publish(ctx, "x", []byte(`{"event":"cycle"}`)); err == nil {
		t.Error("Publish to a silent server succeeded, want an error")
	}

	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("Publish took %v with a 200ms context, want it to give up with the context", took)
	}
}

func TestComposeWordsEachEvent(t *testing.T) {
	tests := []struct {
		name, payload, title, body string
	}{
		{"lifecycle", `{"event":"lifecycle","instance":"home","state":"started","version":"0.5.0"}`, "dnspatch: home", "started 0.5.0"},
		{"cycle failure", `{"event":"cycle","instance":"home","success":false,"error":"boom"}`, "dnspatch: home", "cycle failed\nerror: boom"},
		{"provider", `{"event":"provider_status","instance":"home","provider":"cf","state":"failure","error":"403"}`, "dnspatch: home", "provider cf: failure\nerror: 403"},
		{"first address", `{"event":"ip_change","instance":"home","changes":[{"provider":"cf","family":"ipv6","new":"::1"}]}`, "dnspatch: home", "cf ipv6: none -> ::1"},
		{"not an event", `plain text`, "dnspatch", "plain text"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			title, body := compose([]byte(tc.payload))
			if title != tc.title || body != tc.body {
				t.Errorf("compose = (%q, %q), want (%q, %q)", title, body, tc.title, tc.body)
			}
		})
	}
}

func TestRedactCutsLongestFirstAndKeepsShortParts(t *testing.T) {
	secrets := urlSecrets("telegram://123456:abcdef@telegram/v1?channels=chatname")
	got := redact("post telegram://123456:abcdef@telegram/v1?channels=chatname failed: v1 chatname", secrets)

	for _, leaked := range []string{"123456", "abcdef", "chatname"} {
		if strings.Contains(got, leaked) {
			t.Errorf("redact left %q in %q", leaked, got)
		}
	}

	if !strings.Contains(got, "v1") {
		t.Errorf("redact cut the short part %q out of %q", "v1", got)
	}

	if len(secrets) == 0 || secrets[0] != "telegram://123456:abcdef@telegram/v1?channels=chatname" {
		t.Error("redact changed or reordered the list it was given")
	}
}
