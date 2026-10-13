package php

import (
	"testing"

	"github.com/JacobJoergensen/laravel-fmt/internal/config"
)

func TestValueStaysOnItsArrowLineWhenPintJoinsIt(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"array value",
			"$a = ['google' => filled(config('services.google.picker.api_key')) && filled(config('services.google.client_id_value_string'))];",
			"$a = [\n    'google' => filled(config('services.google.picker.api_key'))\n" +
				"        && filled(config('services.google.client_id_value_string')),\n];",
		},
		{
			"match result",
			"$a = match ($p) {\n    self::GoogleDrive => filled(config('services.google.picker.api_key')) && filled(config('services.google.client_id')),\n};",
			"$a = match ($p) {\n    self::GoogleDrive => filled(config('services.google.picker.api_key'))\n" +
				"        && filled(config('services.google.client_id')),\n};",
		},
	}

	cfg := config.Default()
	cfg.PHP.ArrowValueOnKeyLine = true

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(mustFormat(t, []byte("<?php\n\n"+tc.src+"\n"), cfg))

			if want := "<?php\n\n" + tc.want + "\n"; got != want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, want)
			}

			if twice := string(mustFormat(t, []byte(got), cfg)); twice != got {
				t.Fatalf("not idempotent.\n=== once ===\n%s\n=== twice ===\n%s", got, twice)
			}
		})
	}
}
