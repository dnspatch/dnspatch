// Package shoutrrr is a notifier that sends every event as a text message
// through Shoutrrr, which reaches Telegram, Discord, Slack, ntfy, Matrix,
// Gotify, e-mail, a webhook and some twenty other services from one service URL
// each; services.go lists them.
//
// The notifier is a transport like the others: it receives the JSON event the
// daemon publishes and turns it into a title and a body a person reads, since
// a chat service has no use for raw JSON. The topic is not used, the service
// URL decides where a message goes.
//
// The URLs carry tokens and passwords, so they are secrets: they are checked
// when the notifier is built, which is at startup, and the errors of a send
// have them cut out before they reach a log.
//
// The package pulls in Shoutrrr and the clients of all its services, which is
// why notifiers are left out of a plain build; see the full and shoutrrr build
// tags.
package shoutrrr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"maps"
	"net/url"
	"slices"
	"strings"

	"github.com/nicholas-fedor/shoutrrr/pkg/types"

	"github.com/dnspatch/dnspatch/plugin"
)

// Name is the type name of the notifier in the configuration file.
const Name = "shoutrrr"

// minSecret is the shortest piece of a URL that is cut out of an error. A
// shorter one, such as a "v1" path segment, is no secret and would make the
// message unreadable.
const minSecret = 4

func init() {
	plugin.RegisterNotifier(Name, newNotifier)
}

// Config holds the parameters of the shoutrrr notifier.
type Config struct {
	URLs []string `toml:"urls,required,secret" example:"[\"telegram://${TELEGRAM_BOT_TOKEN}@telegram?chats=@channel-name\"]" doc:"Shoutrrr service URLs, one per destination, such as telegram://token@telegram?chats=@channel or ntfy://ntfy.sh/topic: every event goes to all of them. Write the token or password of a URL as ${NAME}, the rest can stay in the file. The services and their URL formats are described at https://shoutrrr.nickfedor.com/services/overview/"`

	plugin.NotifierCommon
}

func newNotifier(cfg Config) (plugin.Notifier, error) {
	if len(cfg.URLs) == 0 {
		return nil, errors.New("urls: at least one service URL is required")
	}

	var (
		secrets  []string
		services []types.Service
	)

	for _, raw := range cfg.URLs {
		secrets = append(secrets, urlSecrets(raw)...)
	}

	for i, raw := range cfg.URLs {
		svc, err := newService(raw)
		if err != nil {
			return nil, fmt.Errorf("urls[%d]: %s", i, redact(err.Error(), secrets))
		}

		services = append(services, svc)
	}

	return &notifier{services: services, secrets: secrets}, nil
}

// newService builds the service a URL names and checks the URL against it, so a
// wrong scheme or a missing parameter fails at startup and not on the first
// event.
func newService(raw string) (types.Service, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("not a URL: %w", err)
	}

	build, ok := services[u.Scheme]
	if !ok {
		return nil, fmt.Errorf("unknown service %q, the known ones are %s", u.Scheme, strings.Join(slices.Sorted(maps.Keys(services)), ", "))
	}

	svc := build()
	if err := svc.Initialize(u, log.New(io.Discard, "", 0)); err != nil {
		return nil, err
	}

	return svc, nil
}

type notifier struct {
	services []types.Service
	secrets  []string
}

// Publish sends the event to every service at the same time and returns when
// all have answered. A service that supports it gets ctx, so its deadline cuts
// the request short; the others are waited for no longer than ctx allows.
func (n *notifier) Publish(ctx context.Context, _ string, payload []byte) error {
	title, body := compose(payload)

	results := make(chan error, len(n.services))

	for _, svc := range n.services {
		go func() {
			params := types.Params{}
			params.SetTitle(title)

			if sender, ok := svc.(types.ContextSender); ok {
				results <- sender.SendContext(ctx, body, &params)
				return
			}

			results <- svc.Send(body, &params)
		}()
	}

	var failed []error

	for range n.services {
		select {
		case err := <-results:
			if err != nil {
				failed = append(failed, errors.New(redact(err.Error(), n.secrets)))
			}
		case <-ctx.Done():
			return errors.Join(append(failed, fmt.Errorf("send: %w", ctx.Err()))...)
		}
	}

	return errors.Join(failed...)
}

func (n *notifier) Close() error { return nil }

// event holds the fields of the JSON event that the text is made of; see the
// payloads of internal/hooks/notify.
type event struct {
	Event     string   `json:"event"`
	Severity  string   `json:"severity"`
	Instance  string   `json:"instance"`
	State     string   `json:"state"`
	Success   *bool    `json:"success"`
	Error     string   `json:"error"`
	Provider  string   `json:"provider"`
	Retriever string   `json:"retriever"`
	Version   string   `json:"version"`
	Changes   []change `json:"changes"`
}

type change struct {
	Provider string `json:"provider"`
	Family   string `json:"family"`
	Old      string `json:"old"`
	New      string `json:"new"`
}

// compose makes the title and the body of a message from an event. A payload
// that is not an event the daemon knows is sent as it is, so that a new kind
// of event is not lost before this notifier learns to word it.
func compose(payload []byte) (title, body string) {
	var ev event
	if err := json.Unmarshal(payload, &ev); err != nil || ev.Event == "" {
		return "dnspatch", string(payload)
	}

	title = "dnspatch"
	if ev.Instance != "" {
		title += ": " + ev.Instance
	}

	var lines []string

	switch ev.Event {
	case "ip_change":
		for _, c := range ev.Changes {
			lines = append(lines, fmt.Sprintf("%s %s: %s -> %s", c.Provider, c.Family, orNone(c.Old), c.New))
		}
	case "lifecycle":
		lines = append(lines, strings.TrimSpace(fmt.Sprintf("%s %s", ev.State, ev.Version)))
	case "provider_status":
		lines = append(lines, fmt.Sprintf("provider %s: %s", ev.Provider, ev.State))
	case "retriever_status":
		lines = append(lines, fmt.Sprintf("retriever %s: %s", ev.Retriever, ev.State))
	case "cycle":
		lines = append(lines, "cycle "+outcome(ev.Success))
	default:
		lines = append(lines, ev.Event+": "+ev.State)
	}

	if ev.Error != "" {
		lines = append(lines, "error: "+ev.Error)
	}

	return title, strings.Join(lines, "\n")
}

func outcome(success *bool) string {
	if success != nil && *success {
		return "succeeded"
	}

	return "failed"
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}

	return s
}

// urlSecrets lists the parts of a service URL that grant access: the login,
// the password, every path segment and every query value. A Shoutrrr URL puts
// its token in any of them depending on the service, and an error of an HTTP
// client repeats the request URL, token included. The scheme and the host are
// left in, they tell which service failed.
func urlSecrets(raw string) []string {
	u, err := url.Parse(raw)
	if err != nil {
		return []string{raw}
	}

	parts := []string{raw}

	if u.User != nil {
		parts = append(parts, u.User.Username())
		if pass, ok := u.User.Password(); ok {
			parts = append(parts, pass)
		}
	}

	for seg := range strings.SplitSeq(u.Path, "/") {
		parts = append(parts, seg)
	}

	for _, values := range u.Query() {
		parts = append(parts, values...)
	}

	return parts
}

// redact replaces each secret in text with "***". The longest go first, so the
// whole URL is cut out before its parts.
func redact(text string, secrets []string) string {
	sorted := slices.SortedFunc(slices.Values(secrets), func(a, b string) int { return len(b) - len(a) })

	for _, s := range sorted {
		if len(s) >= minSecret {
			text = strings.ReplaceAll(text, s, "***")
		}
	}

	return text
}
