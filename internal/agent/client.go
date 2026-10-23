package agent

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mby/fenghuo/internal/proto"
	"github.com/mby/fenghuo/internal/version"
)

var ErrUnauthorized = errors.New("token rejected by server")

type Client struct {
	endpoint string
	token    string
	hc       *http.Client
}

func NewClient(server, token string, allowHTTP bool) (*Client, error) {
	u, err := url.Parse(server)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("invalid server URL %q", server)
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !allowHTTP && !isLoopback(u.Hostname()) {
			return nil, errors.New("refusing to send the token over plain http; use https or pass -allow-http")
		}
	default:
		return nil, fmt.Errorf("unsupported URL scheme %q", u.Scheme)
	}
	u.User, u.RawQuery, u.Fragment = nil, "", ""
	u.Path = strings.TrimRight(u.Path, "/") + "/api/v1/report"

	tr := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		MaxIdleConns:          2,
		IdleConnTimeout:       90 * time.Second,
	}
	return &Client{
		endpoint: u.String(),
		token:    token,
		hc: &http.Client{
			Timeout:   10 * time.Second,
			Transport: tr,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Send posts a report. watched is false only when the server answered
// proto.StatusIdle; any other success keeps the normal interval.
func (c *Client) Send(ctx context.Context, rep *proto.Report) (watched bool, err error) {
	body, err := json.Marshal(rep)
	if err != nil {
		return true, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return true, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("User-Agent", "fenghuo-agent/"+version.Version)

	resp, err := c.hc.Do(req)
	if err != nil {
		return true, err
	}
	// The response body is never interpreted; it is drained only so that the
	// keep-alive connection can be reused.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	resp.Body.Close()

	switch {
	case resp.StatusCode == proto.StatusIdle:
		return false, nil
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return true, nil
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return true, ErrUnauthorized
	default:
		return true, fmt.Errorf("server responded %s", resp.Status)
	}
}
