package tui

import (
	"testing"

	"anime-tracker/internal/db"
	"anime-tracker/internal/qbt"
)

// TestRssState_MarkArticleRead covers the case an article's own copy
// exists in two groups at once — the synthetic "Unread" aggregate and its
// real feed — both of which markArticleRead must update in place along
// with each group's Unread count.
func TestRssState_MarkArticleRead(t *testing.T) {
	shared := qbt.RSSArticle{ID: "a1", FeedName: "Feed1"}
	other := qbt.RSSArticle{ID: "a2", FeedName: "Feed1"}
	r := rssState{
		feeds: []rssFeedGroup{
			{Name: "Unread", Unread: 2, Articles: []qbt.RSSArticle{shared, other}},
			{Name: "Feed1", Unread: 2, Articles: []qbt.RSSArticle{shared, other}},
		},
	}

	r.markArticleRead("a1")

	for gi, g := range r.feeds {
		if g.Unread != 1 {
			t.Errorf("group %d (%s) Unread = %d, want 1", gi, g.Name, g.Unread)
		}
		if !g.Articles[0].IsRead {
			t.Errorf("group %d (%s): article a1 should be marked read", gi, g.Name)
		}
		if g.Articles[1].IsRead {
			t.Errorf("group %d (%s): article a2 should be untouched", gi, g.Name)
		}
	}

	// Marking an already-read article again must not double-decrement Unread.
	r.markArticleRead("a1")
	if r.feeds[0].Unread != 1 {
		t.Errorf("re-marking an already-read article changed Unread to %d, want 1", r.feeds[0].Unread)
	}

	// Unknown id: no-op.
	r.markArticleRead("does-not-exist")
	if r.feeds[0].Unread != 1 {
		t.Errorf("marking an unknown id changed Unread to %d, want 1", r.feeds[0].Unread)
	}
}

func TestIndexByID(t *testing.T) {
	items := []db.Episode{{ID: 10}, {ID: 20}, {ID: 30}}
	keyFn := func(e db.Episode) int64 { return e.ID }

	if got := indexByID(items, 20, keyFn, 99); got != 1 {
		t.Errorf("indexByID found existing id: got %d, want 1", got)
	}
	if got := indexByID(items, 999, keyFn, 2); got != 2 {
		t.Errorf("indexByID missing id, in-bounds fallback: got %d, want 2", got)
	}
	if got := indexByID(items, 999, keyFn, 50); got != 2 {
		t.Errorf("indexByID missing id, out-of-bounds fallback clamped: got %d, want 2", got)
	}
	if got := indexByID([]db.Episode{}, 1, keyFn, 5); got != 0 {
		t.Errorf("indexByID empty list: got %d, want 0", got)
	}
}
