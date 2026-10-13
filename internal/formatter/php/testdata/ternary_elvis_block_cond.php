<?php

$result = match ($status) {
    'active' => loadActive(),
    'pending' => loadPending(),
    default => null,
}
    ?: $fallback;

$items = [
    'first',
    'second',
]
    ?: $defaults;
