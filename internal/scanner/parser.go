package scanner

import (
	"regexp"
	"strconv"
)

var (
	reSxxExx      = regexp.MustCompile(`(?i)s(\d{1,2})e(\d{1,3})`)
	reDashTrail   = regexp.MustCompile(`\s-\s*(\d{1,3})(?:v\d+)?(?:\.\d+)?(?:\s|\.|\[|\(|$)`)
	reDashLead    = regexp.MustCompile(`^\[.*?\]\s*(?:.*?\s)?(\d{1,3})\s*-\s`)
	reEpisodeWord = regexp.MustCompile(`(?i)\bep?(?:isode)?\.?\s*(\d{1,4})\b`)
	reFallbackNum = regexp.MustCompile(`\b(\d{2,4})\b`)
)

// tokens that look like episode numbers but aren't (resolutions/codecs).
var excludedTokens = map[string]struct{}{
	"1080": {}, "720": {}, "480": {}, "360": {}, "240": {}, "2160": {},
	"264": {}, "265": {},
}

// ParseEpisodeNumber extracts an episode number from a filename, trying
// progressively looser patterns. Returns nil if nothing matched.
func ParseEpisodeNumber(filename string) *int {
	if m := reSxxExx.FindStringSubmatch(filename); m != nil {
		return atoiPtr(m[2])
	}

	if matches := reDashTrail.FindAllStringSubmatch(filename, -1); len(matches) > 0 {
		return atoiPtr(matches[len(matches)-1][1])
	}
	if m := reDashLead.FindStringSubmatch(filename); m != nil {
		return atoiPtr(m[1])
	}

	if m := reEpisodeWord.FindStringSubmatch(filename); m != nil {
		return atoiPtr(m[1])
	}

	for _, m := range reFallbackNum.FindAllString(filename, -1) {
		if isExcludedToken(m) {
			continue
		}
		return atoiPtr(m)
	}

	return nil
}

func isExcludedToken(tok string) bool {
	if _, ok := excludedTokens[tok]; ok {
		return true
	}
	if n, err := strconv.Atoi(tok); err == nil && n >= 1900 && n <= 2099 {
		return true
	}
	return false
}

func atoiPtr(s string) *int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return &n
}
