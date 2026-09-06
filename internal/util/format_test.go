package util

import (
	"testing"
	"time"
)

func TestCompactDuration(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{time.Hour, "1h"},
		{24 * time.Hour, "24h"},
		{90 * time.Minute, "1h30m"},
		{30 * time.Minute, "30m"},
		{24*time.Hour + 30*time.Minute, "24h30m"},
		// 含秒：回退到默认 String()
		{time.Hour + 5*time.Second, "1h0m5s"},
		{30 * time.Second, "30s"},
	}
	for _, c := range cases {
		if got := CompactDuration(c.in); got != c.want {
			t.Errorf("CompactDuration(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
