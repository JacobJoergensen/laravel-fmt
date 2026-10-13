package xml

import (
	"fmt"
	"strings"
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

// genSitemap emits a sitemap the way generators do, all on one line, where
// re-homing an element's text by scanning back to the previous newline would
// cost O(n^2). The newline-separated twin is the control, so a regression shows
// as the two curves diverging.
func genSitemap(urls int, oneLine bool) []byte {
	var sb strings.Builder

	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	sb.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)

	for i := range urls {
		fmt.Fprintf(&sb, "<url><loc>https://example.test/page/%d</loc>"+
			"<lastmod>2026-07-29</lastmod><changefreq>daily</changefreq>"+
			"<priority>0.8</priority></url>", i)

		if !oneLine {
			sb.WriteString("\n")
		}
	}

	sb.WriteString("</urlset>")

	return []byte(sb.String())
}

func BenchmarkFormat(b *testing.B) {
	cfg := config.Default()

	cases := []struct {
		name    string
		urls    int
		oneLine bool
	}{
		{"one_line/small", 500, true},
		{"one_line/large", 8000, true},
		{"newlines/large", 8000, false},
	}

	for _, tc := range cases {
		src := genSitemap(tc.urls, tc.oneLine)

		b.Run(tc.name, func(b *testing.B) {
			b.SetBytes(int64(len(src)))
			b.ReportAllocs()

			for b.Loop() {
				if _, err := Format(src, cfg); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
