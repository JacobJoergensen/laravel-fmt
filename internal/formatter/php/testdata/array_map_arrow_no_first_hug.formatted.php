<?php

$wallClocks = array_map(
    static fn (array $o): string => $o['date']->copy()->setTimezone('Europe/Berlin')->format('Y-m-d H:i'),
    $occurrences,
);
