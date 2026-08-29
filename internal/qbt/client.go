// Package qbt is a minimal client for the qBittorrent WebUI API (v2),
// covering just what anime-tracker needs: log in, add a torrent by
// magnet/URL, list torrents by tag, and remove a tag once synced.
package qbt

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
)

// Torrent is the subset of qBittorrent's torrent info this package cares
// about.
type Torrent struct {
	Hash        string
	Name        string
	State       string
	Progress    float64 // 0..1
	SavePath    string
	ContentPath string
}

// Client talks to one qBittorrent WebUI instance. Callers must call Login
// before any other method.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// New creates a Client for the qBittorrent WebUI at baseURL (e.g.
// "https://seedbox.example.com:8080"). Set insecureSkipVerify to accept a
// self-signed TLS certificate.
func New(baseURL string, insecureSkipVerify bool) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("creating cookie jar: %w", err)
	}
	transport := &http.Transport{}
	if insecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // explicit opt-in via config
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Jar: jar, Transport: transport},
	}, nil
}

// Login authenticates and stores the resulting session cookie for
// subsequent calls.
func (c *Client) Login(ctx context.Context, username, password string) error {
	form := url.Values{"username": {username}, "password": {password}}
	body, status, err := c.post(ctx, "/api/v2/auth/login", form)
	if err != nil {
		return fmt.Errorf("logging in: %w", err)
	}
	if status != http.StatusOK || strings.TrimSpace(body) != "Ok." {
		return fmt.Errorf("login rejected (check qbt.url/qbt.username/qbt.password): %s", strings.TrimSpace(body))
	}
	return nil
}

// AddTorrent submits a magnet link or direct .torrent URL for download,
// saved under savePath and tagged with tags (comma-separated).
func (c *Client) AddTorrent(ctx context.Context, torrentURL, savePath, tags string) error {
	form := url.Values{"urls": {torrentURL}, "savepath": {savePath}, "tags": {tags}}
	body, status, err := c.post(ctx, "/api/v2/torrents/add", form)
	if err != nil {
		return fmt.Errorf("adding torrent: %w", err)
	}
	if status != http.StatusOK {
		return fmt.Errorf("adding torrent: unexpected status %d: %s", status, strings.TrimSpace(body))
	}
	if strings.TrimSpace(body) == "Fails." {
		return fmt.Errorf("qBittorrent rejected the torrent (unsupported link or duplicate)")
	}
	return nil
}

// ListTorrents returns torrents carrying tag.
func (c *Client) ListTorrents(ctx context.Context, tag string) ([]Torrent, error) {
	q := url.Values{"tag": {tag}}
	body, status, err := c.get(ctx, "/api/v2/torrents/info?"+q.Encode())
	if err != nil {
		return nil, fmt.Errorf("listing torrents: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("listing torrents: unexpected status %d: %s", status, strings.TrimSpace(body))
	}

	var raw []struct {
		Hash        string  `json:"hash"`
		Name        string  `json:"name"`
		State       string  `json:"state"`
		Progress    float64 `json:"progress"`
		SavePath    string  `json:"save_path"`
		ContentPath string  `json:"content_path"`
	}
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		return nil, fmt.Errorf("decoding torrent list: %w", err)
	}

	out := make([]Torrent, len(raw))
	for i, t := range raw {
		out[i] = Torrent{
			Hash:        t.Hash,
			Name:        t.Name,
			State:       t.State,
			Progress:    t.Progress,
			SavePath:    t.SavePath,
			ContentPath: t.ContentPath,
		}
	}
	return out, nil
}

// RemoveTags strips tags (comma-separated) from the given torrents.
func (c *Client) RemoveTags(ctx context.Context, hashes []string, tags string) error {
	form := url.Values{"hashes": {strings.Join(hashes, "|")}, "tags": {tags}}
	body, status, err := c.post(ctx, "/api/v2/torrents/removeTags", form)
	if err != nil {
		return fmt.Errorf("removing tags: %w", err)
	}
	if status != http.StatusOK {
		return fmt.Errorf("removing tags: unexpected status %d: %s", status, strings.TrimSpace(body))
	}
	return nil
}

func (c *Client) post(ctx context.Context, path string, form url.Values) (body string, status int, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.do(req)
}

func (c *Client) get(ctx context.Context, path string) (body string, status int, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return "", 0, err
	}
	return c.do(req)
}

// do sends req with the Referer/Origin headers qBittorrent's WebUI CSRF
// check expects, and reads the full response body.
func (c *Client) do(req *http.Request) (body string, status int, err error) {
	req.Header.Set("Referer", c.baseURL)
	req.Header.Set("Origin", c.baseURL)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", resp.StatusCode, fmt.Errorf("reading response: %w", err)
	}
	return string(b), resp.StatusCode, nil
}
