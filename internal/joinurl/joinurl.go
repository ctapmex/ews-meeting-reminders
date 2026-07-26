package joinurl

import (
	"net/url"
	"regexp"
	"strings"
)

var urlRe = regexp.MustCompile(`(?i)https?://[^\s<>"')\]]+`)

func HostMatches(host, pattern string) bool {
	host = strings.ToLower(host)
	if at := strings.LastIndex(host, "@"); at >= 0 {
		host = host[at+1:]
	}
	if i := strings.LastIndex(host, ":"); i >= 0 {
		host = host[:i]
	}
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	switch {
	case strings.HasPrefix(pattern, "*."):
		suffix := pattern[2:]
		return host == suffix || strings.HasSuffix(host, "."+suffix)
	case strings.HasPrefix(pattern, "."):
		return strings.HasSuffix(host, pattern) || host == pattern[1:]
	default:
		return host == pattern || strings.HasSuffix(host, "."+pattern)
	}
}

func IsJoinURL(raw string, joinHosts []string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, p := range joinHosts {
		if HostMatches(host, p) {
			return true
		}
	}
	return false
}

func stripTrailingPunct(s string) string {
	return strings.TrimRight(s, ".,;:!?)>]\"'")
}

func isHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	return scheme == "http" || scheme == "https"
}

// Extract picks a join URL: any http(s) URL from Location first;
// otherwise a body URL whose host matches joinHosts.
func Extract(location, body string, joinHosts []string) string {
	loc := strings.TrimSpace(location)
	if loc != "" {
		if strings.HasPrefix(strings.ToLower(loc), "http://") || strings.HasPrefix(strings.ToLower(loc), "https://") {
			first := stripTrailingPunct(strings.Fields(loc)[0])
			if isHTTPURL(first) {
				return first
			}
		}
		for _, m := range urlRe.FindAllString(loc, -1) {
			c := stripTrailingPunct(m)
			if isHTTPURL(c) {
				return c
			}
		}
	}
	for _, m := range urlRe.FindAllString(body, -1) {
		c := stripTrailingPunct(m)
		if IsJoinURL(c, joinHosts) {
			return c
		}
	}
	return ""
}
