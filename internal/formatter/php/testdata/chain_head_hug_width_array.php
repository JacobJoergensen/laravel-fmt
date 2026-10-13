<?php

function build(): void
{
    $sourceLine = static fn ($item): string => collect([
        $item->source?->value,
        $item->attribution,
        $item->source_url,
        $item->external_reference,
    ])
        ->filter()
        ->implode(' · ');
}
