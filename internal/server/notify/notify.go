// Package notify delivers alert messages to Telegram and generic webhooks.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/mby/fenghuo/internal/server/store"
)

type Message struct {
	Title  string `json:"title"`
	Text   string `json:"text"`
	Node   string `json:"node"`
	Kind   string `json:"kind"`
	Firing bool   `json:"firing"`
	Time   int64  `json:"time"`
}

type Notifier struct {
	st      *store.Store
	hc      *http.Client
	log     *slog.Logger
	running atomic.Bool
}

func New(st *store.Store, allowPrivate bool, log *slog.Logger) *Notifier {
	if log == nil {
		log = slog.Default()
	}
	return &Notifier{st: st, hc: newHTTPClient(allowPrivate), log: log}
}

// Broadcast durably queues msg. Run owns all network activity and goroutines.
// Call Enqueue when the caller needs to handle capacity or persistence errors.
func (n *Notifier) Broadcast(msg Message) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := n.Enqueue(ctx, msg); err != nil {
		n.log.Error("notification enqueue failed", "queue_full", errors.Is(err, store.ErrDeliveryQueueFull))
	}
}

func (n *Notifier) Send(ctx context.Context, ch store.Channel, msg Message) error {
	raw, err := ValidateConfig(ch.Type, ch.Config)
	if err != nil {
		return errors.New("invalid_config")
	}
	switch ch.Type {
	case "telegram":
		var c TelegramConfig
		if err := json.Unmarshal(raw, &c); err != nil {
			return errors.New("invalid_config")
		}
		return n.sendTelegram(ctx, c, msg)
	case "webhook":
		var c WebhookConfig
		if err := json.Unmarshal(raw, &c); err != nil {
			return errors.New("invalid_config")
		}
		return n.sendWebhook(ctx, c, msg)
	default:
		return errors.New("invalid_config")
	}
}

type TelegramConfig struct {
	BotToken string `json:"bot_token"`
	ChatID   string `json:"chat_id"`
	// APIBase allows a self-hosted Bot API or reverse proxy.
	APIBase string `json:"api_base,omitempty"`
}

type WebhookConfig struct {
	URL     string            `json:"url"`
	Method  string            `json:"method,omitempty"` // POST (default) or GET
	Headers map[string]string `json:"headers,omitempty"`
	// Body is an optional template; when empty the message is sent as JSON.
	Body string `json:"body,omitempty"`
}

var botTokenRe = regexp.MustCompile(`^[0-9]{5,16}:[A-Za-z0-9_-]{20,64}$`)

// ValidateConfig checks and normalises a channel configuration.
func ValidateConfig(typ string, raw json.RawMessage) (json.RawMessage, error) {
	switch typ {
	case "telegram":
		var c TelegramConfig
		if err := strictUnmarshal(raw, &c); err != nil {
			return nil, err
		}
		c.BotToken, c.ChatID, c.APIBase = strings.TrimSpace(c.BotToken), strings.TrimSpace(c.ChatID), strings.TrimSpace(c.APIBase)
		if !botTokenRe.MatchString(c.BotToken) {
			return nil, errors.New("invalid telegram bot token")
		}
		if c.ChatID == "" || len(c.ChatID) > 64 {
			return nil, errors.New("invalid telegram chat id")
		}
		if c.APIBase != "" {
			if err := checkURL(c.APIBase); err != nil {
				return nil, err
			}
			c.APIBase = strings.TrimRight(c.APIBase, "/")
		}
		return json.Marshal(c)
	case "webhook":
		var c WebhookConfig
		if err := strictUnmarshal(raw, &c); err != nil {
			return nil, err
		}
		c.URL = strings.TrimSpace(c.URL)
		if err := checkURL(c.URL); err != nil {
			return nil, err
		}
		c.Method = strings.ToUpper(strings.TrimSpace(c.Method))
		if c.Method == "" {
			c.Method = http.MethodPost
		}
		if c.Method != http.MethodPost && c.Method != http.MethodGet {
			return nil, errors.New("webhook method must be GET or POST")
		}
		if len(c.Headers) > 16 {
			return nil, errors.New("too many webhook headers")
		}
		for k, v := range c.Headers {
			if k == "" || strings.ContainsAny(k+v, "\r\n") || len(k)+len(v) > 1024 {
				return nil, fmt.Errorf("invalid webhook header %q", k)
			}
		}
		if len(c.Body) > 8<<10 {
			return nil, errors.New("webhook body template too long")
		}
		return json.Marshal(c)
	default:
		return nil, fmt.Errorf("unknown channel type %q", typ)
	}
}

func strictUnmarshal(raw json.RawMessage, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("invalid config: unexpected trailing data")
	}
	return nil
}

func checkURL(s string) error {
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return errors.New("invalid URL (must be http or https)")
	}
	return nil
}

func (n *Notifier) sendTelegram(ctx context.Context, c TelegramConfig, msg Message) error {
	base := c.APIBase
	if base == "" {
		base = "https://api.telegram.org"
	}
	body, _ := json.Marshal(map[string]any{
		"chat_id":                  c.ChatID,
		"text":                     msg.Title + "\n" + msg.Text,
		"disable_web_page_preview": true,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/bot"+c.BotToken+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return errors.New("invalid_config")
	}
	req.Header.Set("Content-Type", "application/json")
	return n.doResponse(req, true)
}

func (n *Notifier) sendWebhook(ctx context.Context, c WebhookConfig, msg Message) error {
	status := "resolved"
	if msg.Firing {
		status = "firing"
	}
	vars := map[string]string{
		"title": msg.Title, "text": msg.Text, "node": msg.Node, "kind": msg.Kind,
		"status": status, "time": time.Unix(msg.Time, 0).Format(time.RFC3339),
	}
	target := render(c.URL, vars, url.PathEscape)

	var body io.Reader
	if c.Method != http.MethodGet {
		var b []byte
		if c.Body == "" {
			b, _ = json.Marshal(msg)
		} else {
			b = []byte(render(c.Body, vars, jsonEscape))
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, c.Method, target, body)
	if err != nil {
		return errors.New("invalid_config")
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range c.Headers {
		req.Header.Set(k, v)
	}
	return n.do(req)
}

func (n *Notifier) do(req *http.Request) error {
	return n.doResponse(req, false)
}

func (n *Notifier) doResponse(req *http.Request, telegram bool) error {
	req.Header.Set("User-Agent", "fenghuo-notify")
	resp, err := n.hc.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return errors.New("canceled")
		}
		var ne net.Error
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()) {
			return errors.New("timeout")
		}
		return errors.New("transport_error")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.New("http_error")
	}
	if telegram {
		var result struct {
			OK bool `json:"ok"`
		}
		if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&result); err != nil || !result.OK {
			return errors.New("telegram_error")
		}
		return nil
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	return nil
}

var placeholderRe = regexp.MustCompile(`\{\{\s*(\w+)\s*\}\}`)

func render(tpl string, vars map[string]string, escape func(string) string) string {
	return placeholderRe.ReplaceAllStringFunc(tpl, func(m string) string {
		key := placeholderRe.FindStringSubmatch(m)[1]
		v, ok := vars[key]
		if !ok {
			return m
		}
		return escape(v)
	})
}

// jsonEscape escapes s for use inside a JSON string literal.
func jsonEscape(s string) string {
	b, _ := json.Marshal(s)
	return string(b[1 : len(b)-1])
}

// newHTTPClient returns a client that, unless allowPrivate is set, refuses to
// connect to loopback, private and link-local addresses. The check runs on the
// resolved address, so DNS tricks and redirects cannot bypass it.
func newHTTPClient(allowPrivate bool) *http.Client {
	d := &net.Dialer{Timeout: 10 * time.Second}
	var proxy func(*http.Request) (*url.URL, error)
	if allowPrivate {
		proxy = http.ProxyFromEnvironment
	}
	if !allowPrivate {
		d.Control = func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip, err := netip.ParseAddr(host)
			if err != nil {
				return err
			}
			if !isPublic(ip) {
				return fmt.Errorf("refusing to connect to non-public address %s (see --notify-allow-private)", ip)
			}
			return nil
		}
	}
	return &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			// A proxy would bypass destination-IP checks in DialContext.
			Proxy:                 proxy,
			DialContext:           d.DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			MaxIdleConns:          4,
			IdleConnTimeout:       60 * time.Second,
		},
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			// Custom webhook credentials must not follow redirects to another host.
			return http.ErrUseLastResponse
		},
	}
}

var cgnat = netip.MustParsePrefix("100.64.0.0/10")

func isPublic(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsValid() && !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast() &&
		!ip.IsLinkLocalMulticast() && !ip.IsInterfaceLocalMulticast() && !ip.IsMulticast() &&
		!ip.IsUnspecified() && !cgnat.Contains(ip)
}
