<?php

function describe(Person $person): string
{
    $label = collect([$person->firstName, $person->middleName, $person->lastName, $person->suffix])
        ->filter()
        ->implode(', ');

    $slug = str($person->displayNameForTheDirectoryListing)
        ->lower()
        ->replace(' ', '-')
        ->limit(40)
        ->append('-profile')
        ->toString();

    $limit = now()->addDays($person->noticePeriodInDays)
        ->startOfDay()
        ->setTimezone($person->timezone)
        ->toIso8601String();

    $short = collect($person->tags)->filter()->values();

    return response()->json(['label' => $label, 'slug' => $slug, 'limit' => $limit, 'short' => $short])
        ->header('X-Kind', 'p');
}
