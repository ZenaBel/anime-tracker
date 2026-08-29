package qbt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLogin(t *testing.T) {
	var gotUser, gotPass, gotReferer string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/auth/login" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		gotUser = r.FormValue("username")
		gotPass = r.FormValue("password")
		gotReferer = r.Header.Get("Referer")
		w.Header().Set("Set-Cookie", "SID=abc123; Path=/")
		w.Write([]byte("Ok."))
	}))
	defer srv.Close()

	c, err := New(srv.URL, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Login(context.Background(), "alice", "hunter2"); err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if gotUser != "alice" || gotPass != "hunter2" {
		t.Fatalf("Login sent user=%q pass=%q", gotUser, gotPass)
	}
	if gotReferer != srv.URL {
		t.Fatalf("Referer = %q, want %q", gotReferer, srv.URL)
	}
}

func TestLogin_Rejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Fails."))
	}))
	defer srv.Close()

	c, err := New(srv.URL, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Login(context.Background(), "alice", "wrong"); err == nil {
		t.Fatal("expected error on rejected login")
	}
}

func TestAddTorrent(t *testing.T) {
	var gotURL, gotSavePath, gotTags string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/torrents/add" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		gotURL = r.FormValue("urls")
		gotSavePath = r.FormValue("savepath")
		gotTags = r.FormValue("tags")
		w.Write([]byte("Ok."))
	}))
	defer srv.Close()

	c, _ := New(srv.URL, false)
	err := c.AddTorrent(context.Background(), "magnet:?xt=urn:btih:abc", "/downloads/Frieren", "anime-tracker")
	if err != nil {
		t.Fatalf("AddTorrent() error = %v", err)
	}
	if gotURL != "magnet:?xt=urn:btih:abc" || gotSavePath != "/downloads/Frieren" || gotTags != "anime-tracker" {
		t.Fatalf("AddTorrent sent url=%q savepath=%q tags=%q", gotURL, gotSavePath, gotTags)
	}
}

func TestAddTorrent_Rejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Fails."))
	}))
	defer srv.Close()

	c, _ := New(srv.URL, false)
	if err := c.AddTorrent(context.Background(), "bogus", "/x", "anime-tracker"); err == nil {
		t.Fatal("expected error when qBittorrent rejects the torrent")
	}
}

func TestListTorrents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/torrents/info" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("tag"); got != "anime-tracker" {
			t.Fatalf("tag filter = %q, want anime-tracker", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
			{"hash":"h1","name":"Frieren - 05","state":"stalledUP","progress":1,"save_path":"/downloads/Frieren","content_path":"/downloads/Frieren/Frieren - 05.mkv"},
			{"hash":"h2","name":"Bleach - 03","state":"downloading","progress":0.42,"save_path":"/downloads/Bleach","content_path":"/downloads/Bleach/Bleach - 03.mkv"}
		]`))
	}))
	defer srv.Close()

	c, _ := New(srv.URL, false)
	got, err := c.ListTorrents(context.Background(), "anime-tracker")
	if err != nil {
		t.Fatalf("ListTorrents() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d torrents, want 2", len(got))
	}
	if got[0].Hash != "h1" || got[0].Progress != 1 || got[0].ContentPath != "/downloads/Frieren/Frieren - 05.mkv" {
		t.Fatalf("torrent[0] = %+v", got[0])
	}
	if got[1].Progress != 0.42 {
		t.Fatalf("torrent[1].Progress = %v, want 0.42", got[1].Progress)
	}
}

func TestRemoveTags(t *testing.T) {
	var gotHashes, gotTags string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/torrents/removeTags" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		gotHashes = r.FormValue("hashes")
		gotTags = r.FormValue("tags")
	}))
	defer srv.Close()

	c, _ := New(srv.URL, false)
	if err := c.RemoveTags(context.Background(), []string{"h1", "h2"}, "anime-tracker"); err != nil {
		t.Fatalf("RemoveTags() error = %v", err)
	}
	if gotHashes != "h1|h2" || gotTags != "anime-tracker" {
		t.Fatalf("RemoveTags sent hashes=%q tags=%q", gotHashes, gotTags)
	}
}

func TestListTorrents_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("Forbidden"))
	}))
	defer srv.Close()

	c, _ := New(srv.URL, false)
	_, err := c.ListTorrents(context.Background(), "anime-tracker")
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("ListTorrents() error = %v, want mention of 403", err)
	}
}

// rssFixture mirrors qBittorrent's real /api/v2/rss/items response shape:
// feeds nested inside a folder, plus one feed at the root, to exercise the
// recursive walk.
const rssFixture = `{
	"https://example.com/root-feed.xml": {
		"uid": "u1",
		"url": "https://example.com/root-feed.xml",
		"title": "Root Feed",
		"articles": [
			{"id":"a1","title":"Frieren - 05 [1080p]","date":"2026-08-20T12:00:00+00:00","torrentURL":"magnet:?xt=urn:btih:aaa","isRead":false},
			{"id":"a2","title":"Frieren - 04 [1080p]","date":"2026-08-13T12:00:00+00:00","torrentURL":"magnet:?xt=urn:btih:bbb","isRead":true}
		]
	},
	"My Folder": {
		"https://example.com/nested-feed.xml": {
			"uid": "u2",
			"url": "https://example.com/nested-feed.xml",
			"title": "Nested Feed",
			"articles": [
				{"id":"a3","title":"Bleach - 10 [1080p]","date":"2026-08-27T12:00:00+00:00","torrentURL":"magnet:?xt=urn:btih:ccc","isRead":false}
			]
		}
	}
}`

func TestListRSSArticles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/rss/items" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("withData"); got != "true" {
			t.Fatalf("withData = %q, want true", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(rssFixture))
	}))
	defer srv.Close()

	c, _ := New(srv.URL, false)
	got, err := c.ListRSSArticles(context.Background())
	if err != nil {
		t.Fatalf("ListRSSArticles() error = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d articles, want 3: %+v", len(got), got)
	}

	byID := make(map[string]RSSArticle, len(got))
	for _, a := range got {
		byID[a.ID] = a
	}

	nested, ok := byID["a3"]
	if !ok {
		t.Fatalf("article a3 (in a folder) missing from result: %+v", got)
	}
	if nested.FeedName != "https://example.com/nested-feed.xml" || nested.Title != "Bleach - 10 [1080p]" || nested.IsRead {
		t.Errorf("nested article = %+v", nested)
	}

	root, ok := byID["a1"]
	if !ok || root.TorrentURL != "magnet:?xt=urn:btih:aaa" || root.IsRead {
		t.Fatalf("root article a1 = %+v (ok=%v)", root, ok)
	}
	if read := byID["a2"]; !read.IsRead {
		t.Fatalf("article a2 should be marked read: %+v", read)
	}
}

func TestSortArticlesNewestFirst(t *testing.T) {
	articles := []RSSArticle{
		{ID: "old", Date: "2026-08-13T12:00:00+00:00"},
		{ID: "unparseable", Date: "not a date"},
		{ID: "new", Date: "2026-08-27T12:00:00+00:00"},
		{ID: "mid", Date: "2026-08-20T12:00:00+00:00"},
	}
	SortArticlesNewestFirst(articles)

	var ids []string
	for _, a := range articles {
		ids = append(ids, a.ID)
	}
	want := []string{"new", "mid", "old", "unparseable"}
	if len(ids) != len(want) {
		t.Fatalf("order = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("order = %v, want %v", ids, want)
		}
	}
}
