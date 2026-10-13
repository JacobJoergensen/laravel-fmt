<?php

$entry = collect($entries)->first(
    fn (array $entry): bool => $entry['kind'] === $kind,
) ?? $entries[0] ?? [];

$entry = collect($entries)->first(
    fn (array $entry): bool => $entry['kind'] === $kind,
) ?? $entriesForTheCurrentReportingPeriodAndOrganization[0] ?? $fallbackEntriesForTheOrganizationAndMoreStillHereNow ?? [];

$total = [
    'a' => 1,
] + $first + $second;

$flat = first($entries) ?? $entries[0] ?? [];

$label = sprintf(
    '%s: %s',
    $entry['kind'],
).' / '.$entry['name'].' / '.$entry['owner'];

$entry = collect($entries)->first(
    fn (array $entry): bool => $entry['kind'] === $kind,
) ?? $fallbackEntriesForTheOrganizationfallbackEntriesForTheOrganizationfallbackEntriesForTheOrganizationfallbackEntries ?? [];
