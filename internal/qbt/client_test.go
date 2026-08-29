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
