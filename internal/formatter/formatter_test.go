package formatter

import "testing"

func TestDetectLanguageByExtension(t *testing.T) {
	cases := []struct {
		path string
		want Language
	}{
		{"a.php", LangPHP},
		{"x.blade.php", LangBlade},
		{"weird.BLADE.PHP", LangBlade},
		{"page.html", LangHTML},
		{"style.css", LangCSS},
		{"README.md", LangMarkdown},
		{"composer.json", LangJSON},
		{"package.json", LangJSON},
		{"tsconfig.json", LangJSON},
		{"tsconfig.app.json", LangJSON},
		{"jsconfig.json", LangJSON},
		{"jsconfig.base.json", LangJSON},
		{"TSCONFIG.JSON", LangJSON},
		{"settings.jsonc", LangJSON},
		{"main.go", LangUnknown},
		{"no-extension", LangUnknown},
		{"dir/nested.blade.php", LangBlade},
		// Blade that renders shell (Envoy) or Markdown (Laravel Boost guidelines
		// and skills) is no HTML template, so no layout applies to it.
		{"Envoy.blade.php", LangUnknown},
		{"deploy/envoy.blade.php", LangUnknown},
		{".ai/guidelines/testing.blade.php", LangUnknown},
		{`C:\app\.ai\skills\deploy\SKILL.blade.php`, LangUnknown},
		{"vendor/acme/pkg/resources/boost/guidelines/core.blade.php", LangUnknown},
		{"resources/views/boost.blade.php", LangBlade},
		{"deploy.yml", LangYAML},
		{"docker-compose.yaml", LangYAML},
		{"phpunit.xml", LangXML},
		{"config.XML", LangXML},
		{"phpunit.xml.dist", LangXML},
		{"behat.yml.dist", LangYAML},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			if got := DetectLanguage(tc.path); got != tc.want {
				t.Fatalf("DetectLanguage(%q)=%d, want %d", tc.path, got, tc.want)
			}
		})
	}
}
