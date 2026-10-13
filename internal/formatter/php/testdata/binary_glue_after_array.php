<?php

$toneClass = [
    'warm' => 'text-amber-500',
    'cold' => 'text-sky-500',
][Palette::tone($reading->icon)] ?? 'text-zinc-400';

$totals = [
    'base' => 1,
] + $other;

$toneClass = [
    'warm' => 'text-amber-500',
][Palette::tone($reading->icon)] ?? Palette::fallbackToneForTheCurrentReadingWhenNothingElseMatchesAtAllHereNow($reading);

$flat = ['warm' => 'text-amber-500'][$tone] ?? 'text-zinc-400';

$empty = [] + $other;

$label = Palette::labels(
    $reading->icon,
    $reading->kind,
)[0] ?? 'text-zinc-400';
