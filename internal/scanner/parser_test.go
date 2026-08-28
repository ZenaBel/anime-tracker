package scanner

import (
	"fmt"
	"testing"
)

func TestParseEpisodeNumber(t *testing.T) {
	cases := []struct {
		filename string
		want     *int
	}{
		{"Attack on Titan - S04E05 - Counterattack.mkv", intPtr(5)},
		{"Bleach.S01E03.720p.mkv", intPtr(3)},
		{"My Hero Academia s5e12.mkv", intPtr(12)},
		{"[SubsPlease] Jujutsu Kaisen - 05 [1080p].mkv", intPtr(5)},
		{"[Erai-raws] Frieren - 12 (1080p) [ABCD1234].mkv", intPtr(12)},
		{"Cowboy Bebop - 26 (Final) [BDRip 1080p].mkv", intPtr(26)},
		{"Death Note 2006 - 01 [BD 1080p].mkv", intPtr(1)},
		{"[Group] Chainsaw Man - 07v2 [1080p].mkv", intPtr(7)},
		{"Spy x Family 01 - Operation Strix.mkv", intPtr(1)},
		{"One Piece EP1071.mkv", intPtr(1071)},
		{"Naruto Shippuden - Episode 200.mkv", intPtr(200)},
		{"Vinland Saga 24.mkv", intPtr(24)},
		{"[Group] Show - 05.5 [1080p].mkv", intPtr(5)},
		{"Show.Title.2160p.WEB-DL.x265-GROUP.mkv", nil},
		{"Mob Psycho 100 II - 05.mkv", intPtr(5)},
		{"Aishiteru_Game_wo_Owarasetai_[01]_[HEVC].mkv", intPtr(1)},
		{"Grand_Blue_Season_2_[11]_[HEVC].mkv", intPtr(11)},
		{"Golden_Kamuy_[09]_[AniLibria_TV]_[HDTV-Rip_720p].mkv", intPtr(9)},
		{"Class_no_Daikirai_na_Joshi_[01]_[AniLibria]_[WEBRip_1080p].mkv", intPtr(1)},
	}

	for _, tc := range cases {
		t.Run(tc.filename, func(t *testing.T) {
			got := ParseEpisodeNumber(tc.filename)
			if (got == nil) != (tc.want == nil) {
				t.Fatalf("ParseEpisodeNumber(%q) = %v, want %v", tc.filename, fmtIntPtr(got), fmtIntPtr(tc.want))
			}
			if got != nil && *got != *tc.want {
				t.Fatalf("ParseEpisodeNumber(%q) = %d, want %d", tc.filename, *got, *tc.want)
			}
		})
	}
}

func intPtr(n int) *int { return &n }

func fmtIntPtr(p *int) string {
	if p == nil {
		return "nil"
	}
	return fmt.Sprintf("%d", *p)
}
