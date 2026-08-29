package cmd

import (
	"context"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"anime-tracker/internal/db"
	"anime-tracker/internal/qbt"
	"anime-tracker/internal/search"
	"anime-tracker/internal/settings"
)

var (
	rssShowAll bool
	rssYes     bool
)

func init() {
	rssListCmd.Flags().BoolVar(&rssShowAll, "all", false, "include already-read articles too")
	rssDownloadCmd.Flags().BoolVarP(&rssYes, "yes", "y", false, "skip the guessed-series confirmation prompt")
}

// fetchRSSArticles reads every RSS article qBittorrent's own RSS reader has
// fetched (see internal/qbt.ListRSSArticles — anime-tracker never parses
// RSS itself), optionally filtered to unread ones, newest first.
func fetchRSSArticles(ctx context.Context, store *db.Store, all bool) ([]qbt.RSSArticle, error) {
	client, err := settings.Connect(ctx, store)
	if err != nil {
		return nil, err
	}
	articles, err := client.ListRSSArticles(ctx)
	if err != nil {
		return nil, err
	}
	if !all {
		var unread []qbt.RSSArticle
		for _, a := range articles {
			if !a.IsRead {
				unread = append(unread, a)
			}
		}
		articles = unread
	}
	qbt.SortArticlesNewestFirst(articles)
	return articles, nil
}

var rssListCmd = &cobra.Command{
	Use:   "rss",
	Short: "List RSS articles qBittorrent's own RSS reader has fetched (unread by default)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, _, closeStore, err := openStore(cmd)
		if err != nil {
			return err
		}
		defer closeStore()

		articles, err := fetchRSSArticles(cmd.Context(), store, rssShowAll)
		if err != nil {
			return err
		}
		if len(articles) == 0 {
			fmt.Println("no articles")
			return nil
		}
		for i, a := range articles {
			read := ""
			if a.IsRead {
				read = " (read)"
			}
			fmt.Printf("%3d  [%s] %s%s\n", i+1, a.FeedName, a.Title, read)
		}
		return nil
	},
}

var rssDownloadCmd = &cobra.Command{
	Use:   "rss-download <article-number> [series-query]",
	Short: "Download an article from `rss`'s listing to the remote qBittorrent",
	Long: `<article-number> refers to the most recent 'rss' listing — re-run 'rss'
first if it's been a while. With [series-query] omitted, the series is
guessed by fuzzy-matching the article's title against your tracked series
and confirmed before downloading (pass -y to skip the confirmation).`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, _, closeStore, err := openStore(cmd)
		if err != nil {
			return err
		}
		defer closeStore()

		ctx := cmd.Context()
		n, err := strconv.Atoi(args[0])
		if err != nil || n < 1 {
			return fmt.Errorf("invalid article number %q", args[0])
		}

		articles, err := fetchRSSArticles(ctx, store, rssShowAll)
		if err != nil {
			return err
		}
		if n > len(articles) {
			return fmt.Errorf("only %d article(s) listed — re-run `rss` first", len(articles))
		}
		article := articles[n-1]

		var series db.SeriesProgress
		if len(args) == 2 {
			series, err = findSeriesByQuery(ctx, store, args[1])
			if err != nil {
				return err
			}
		} else {
			allSeries, err := store.ListSeriesWithProgress(ctx, db.SortAlphaAsc)
			if err != nil {
				return err
			}
			guess, ok := search.GuessSeriesForTitle(allSeries, article.Title)
			if !ok {
				return fmt.Errorf("couldn't guess a series from %q — pass a series query explicitly", article.Title)
			}
			series = guess
			if !rssYes && !confirm(fmt.Sprintf("Guessed series %q for %q — download?", series.Title, article.Title)) {
				fmt.Println("cancelled")
				return nil
			}
		}

		savePath, err := settings.RemoteSeriesSavePath(ctx, store, series.Title)
		if err != nil {
			return err
		}
		client, err := settings.Connect(ctx, store)
		if err != nil {
			return err
		}
		if err := client.AddTorrent(ctx, article.TorrentURL, savePath, qbt.Tag); err != nil {
			return err
		}

		fmt.Printf("queued %q for %s, saving to %s\n", article.Title, series.Title, savePath)
		return nil
	},
}
