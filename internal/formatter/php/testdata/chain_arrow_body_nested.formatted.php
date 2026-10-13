<?php

$anyMatchingDay = collect($buckets)->some(
    fn (Carbon $day) => $registry->entriesByKey->get($day->format('Y-m-d'), collect())
        ->contains(fn (Entry $entry) => $entry->enabled),
);
