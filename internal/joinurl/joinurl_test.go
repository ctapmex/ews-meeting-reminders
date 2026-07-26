package joinurl

import "testing"

func TestExtract(t *testing.T) {
	hosts := []string{"*.ktalk.ru", "*.ktalks.tu", "trueconf.x.com", "zoom.us", "*.zoom.us"}
	cases := []struct {
		loc, body, want string
	}{
		{"https://x.ktalk.ru/111", "", "https://x.ktalk.ru/111"},
		{"Room", "join https://trueconf.x.com/c/1", "https://trueconf.x.com/c/1"},
		{"https://us05web.zoom.us/j/1", "", "https://us05web.zoom.us/j/1"},
		// Location URL is accepted regardless of join_hosts.
		{"https://evil.example/x", "https://evil.example/y", "https://evil.example/x"},
		{"Room A https://any.domain/meet", "", "https://any.domain/meet"},
		// Body-only unknown host is ignored.
		{"Переговорка", "https://evil.example/y", ""},
		{"Переговорка", "see https://x.ktalk.ru/bss", "https://x.ktalk.ru/bss"},
		{"", "", ""},
	}
	for _, tc := range cases {
		got := Extract(tc.loc, tc.body, hosts)
		if got != tc.want {
			t.Fatalf("Extract(%q,%q)=%q want %q", tc.loc, tc.body, got, tc.want)
		}
	}
}
