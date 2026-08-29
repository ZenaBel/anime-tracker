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
	"sort"
	"strings"
	"time"
)

// Tag marks every torrent anime-tracker itself cares about — both ones it
// added directly (via download) and ones the user's own qBittorrent RSS
// Auto Downloading rules tag the same way — so sync-downloads can find
// them regardless of how they were added.
const Tag = "anime-tracker"

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
// subsequent calls. Success is judged primarily by whether a session
// cookie actually landed in the jar, not by status code or body text —
// qBittorrent's WebUI has answered a successful login differently across
// versions/configs seen in the wild (200 with body "Ok.", or 204 with an
// empty body); the cookie is the one thing that's actually meant to be
// there either way. The status/body check is only a fallback for the
// unlikely case the cookie didn't get set for some other reason.
func (c *Client) Login(ctx context.Context, username, password string) error {
	form := url.Values{"username": {username}, "password": {password}}
	body, status, err := c.post(ctx, "/api/v2/auth/login", form)
	if err != nil {
		return fmt.Errorf("logging in: %w", err)
	}

	if u, uErr := url.Parse(c.baseURL); uErr == nil && c.httpClient.Jar != nil {
		for _, ck := range c.httpClient.Jar.Cookies(u) {
			if strings.Contains(ck.Name, "SID") {
				return nil
			}
		}
	}

	if status >= 200 && status < 300 && strings.TrimSpace(body) != "Fails." {
		return nil
	}
	return fmt.Errorf("login rejected (check qbt.url/qbt.username/qbt.password): status %d: %s", status, strings.TrimSpace(body))
}

// AddTorrent submits a magnet link or direct .torrent URL for download,
// saved under savePath and tagged with tags (comma-separated).
//
// Response shape has varied across qBittorrent versions in the wild: older
// ones answer with plain "Ok."/"Fails." text and status 200; newer ones
// answer with a JSON summary (added/pending/failure counts) and can use
// 202 Accepted when the torrent is only queued for processing so far (e.g.
// still resolving a magnet's metadata) rather than added outright. Both
// are treated as success unless something actually reports a failure.
func (c *Client) AddTorrent(ctx context.Context, torrentURL, savePath, tags string) error {
	form := url.Values{"urls": {torrentURL}, "savepath": {savePath}, "tags": {tags}}
	body, status, err := c.post(ctx, "/api/v2/torrents/add", form)
	if err != nil {
		return fmt.Errorf("adding torrent: %w", err)
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("adding torrent: unexpected status %d: %s", status, strings.TrimSpace(body))
	}

	var summary struct {
		FailureCount int `json:"failure_count"`
	}
	if jsonErr := json.Unmarshal([]byte(body), &summary); jsonErr == nil {
		if summary.FailureCount > 0 {
			return fmt.Errorf("qBittorrent rejected the torrent: %s", strings.TrimSpace(body))
		}
		return nil
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

// RSSArticle is one item from a subscribed RSS feed, as qBittorrent's own
// RSS reader already fetched and parsed it. anime-tracker never fetches or
// parses RSS itself.
type RSSArticle struct {
	FeedName   string
	ID         string
	Title      string
	TorrentURL string
	Date       string
	IsRead     bool
}

// ListRSSArticles returns every article across every RSS feed (and folder
// of feeds) qBittorrent is subscribed to.
func (c *Client) ListRSSArticles(ctx context.Context) ([]RSSArticle, error) {
	body, status, err := c.get(ctx, "/api/v2/rss/items?withData=true")
	if err != nil {
		return nil, fmt.Errorf("listing RSS articles: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("listing RSS articles: unexpected status %d: %s", status, strings.TrimSpace(body))
	}

	var root map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &root); err != nil {
		return nil, fmt.Errorf("decoding RSS items: %w", err)
	}

	var out []RSSArticle
	walkRSSNode(root, &out)
	return out, nil
}

// rssFeedNode and rssArticleJSON mirror qBittorrent's RSS item JSON shape.
// A feed node carries an "articles" key (even if empty); anything else is
// a folder holding more nodes (feeds can be organized into folders), so
// the tree is walked recursively to flatten it.
type rssFeedNode struct {
	Articles []rssArticleJSON `json:"articles"`
}

type rssArticleJSON struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Date       string `json:"date"`
	TorrentURL string `json:"torrentURL"`
	IsRead     bool   `json:"isRead"`
}

func walkRSSNode(node map[string]json.RawMessage, out *[]RSSArticle) {
	for name, raw := range node {
		var feed rssFeedNode
		if err := json.Unmarshal(raw, &feed); err == nil && feed.Articles != nil {
			for _, a := range feed.Articles {
				*out = append(*out, RSSArticle{
					FeedName:   name,
					ID:         a.ID,
					Title:      a.Title,
					TorrentURL: a.TorrentURL,
					Date:       a.Date,
					IsRead:     a.IsRead,
				})
			}
			continue
		}
		var folder map[string]json.RawMessage
		if err := json.Unmarshal(raw, &folder); err == nil {
			walkRSSNode(folder, out)
		}
	}
}

// SortArticlesNewestFirst sorts articles by date, most recent first;
// articles whose date can't be parsed sink to the end.
func SortArticlesNewestFirst(articles []RSSArticle) {
	sort.SliceStable(articles, func(i, j int) bool {
		ti, oki := parseRSSDate(articles[i].Date)
		tj, okj := parseRSSDate(articles[j].Date)
		if !oki {
			return false
		}
		if !okj {
			return true
		}
		return ti.After(tj)
	})
}

func parseRSSDate(s string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339, time.RFC1123Z, time.RFC1123, time.RFC822Z} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
