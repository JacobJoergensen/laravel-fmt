<?php

$query->where(function ($group) {
    $group->where('status', 'active')
        ->where('type', 'standard')
        ->whereBetween('published_at', [$rangeStart, $rangeEnd])
        ->orderByDesc('published_at');
})->orWhere(function ($group) {
    $group->where('type', 'featured')->whereNotNull('promoted_at');
});
