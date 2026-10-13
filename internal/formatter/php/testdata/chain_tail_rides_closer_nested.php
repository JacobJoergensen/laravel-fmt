<?php

Foo::get('/x.json', fn () => bar()->json([
    'name' => 'A',
    'short_name' => 'B',
])->header('Content-Type', 'application/manifest+json')
    ->header('Cache-Control', 'public, max-age=3600'))->name('x');

$f = fn () => bar()->json([
    'name' => 'A',
    'short_name' => 'B',
])->header('Content-Type', 'application/manifest+json')
    ->header('Cache-Control', 'public, max-age=3600');

$g = fn () => bar()->json([
    'name' => 'A',
    'short_name' => 'B',
])
    ->header('Content-Type', 'application/manifest+json')
    ->header('Cache-Control', 'public, max-age=3600');
