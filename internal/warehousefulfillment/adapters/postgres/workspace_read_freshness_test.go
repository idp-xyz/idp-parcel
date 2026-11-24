package postgres

import (
	"testing"
	"time"
)

func TestStatementFreshnessMatchesReadStatement(t *testing.T) {
	until := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name       string
		now        time.Time
		superseded bool
		want       string
	}{
		{name: "仍在有效期内", now: until.Add(-time.Second), want: "VALID"},
		{name: "刚好到有效期末", now: until, want: "UNKNOWN"},
		{name: "已过有效期末", now: until.Add(time.Second), want: "UNKNOWN"},
		{name: "同一来源有更晚声明", now: until.Add(-time.Second), superseded: true, want: "SUPERSEDED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := statementFreshness(tc.now, until, tc.superseded); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}
