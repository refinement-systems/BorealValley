package main

import "testing"

func TestSanitizeReturnTo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty returns empty", raw: "", want: ""},
		{name: "absolute path passes through", raw: "/web/admin", want: "/web/admin"},
		{name: "root path passes", raw: "/", want: "/"},
		{name: "rejects non-slash start", raw: "http://evil.com", want: ""},
		{name: "rejects double slash", raw: "//evil.com", want: ""},
		{name: "rejects backslash second char", raw: "/\\evil.com", want: ""},
		{name: "rejects tab after slash", raw: "/\tevil.com", want: ""},
		{name: "rejects newline after slash", raw: "/\nevil.com", want: ""},
		{name: "rejects carriage return after slash", raw: "/\revil.com", want: ""},
		{name: "plain relative is rejected", raw: "evil.com", want: ""},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := sanitizeReturnTo(tc.raw)
			if got != tc.want {
				t.Fatalf("sanitizeReturnTo(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}
