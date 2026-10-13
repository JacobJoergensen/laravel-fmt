<?php

class Locale
{
    public function parse(string $header): Collection
    {
        $locales = collect(explode(',', $header))->map(fn (string $locale) => mb_strtolower(mb_substr(mb_trim($locale), 0, 2)));

        return $locales;
    }
}
