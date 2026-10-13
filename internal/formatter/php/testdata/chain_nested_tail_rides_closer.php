<?php

Router::get('/manifest.json', fn () => response()->json([
    'name' => 'Example',
    'display' => 'standalone',
])->header('Content-Type', 'application/manifest+json')->header('Cache-Control', 'public, max-age=3600'))->name('manifest');

register(make()->with([
    'alpha' => 'first',
    'beta' => 'second',
])->label('x'));

$handler = fn () => make()->with([
    'alpha' => 'first',
    'beta' => 'second',
])->label('a label long enough that it cannot ride the closer of the array above at all, not even close at all')->tag('yy');

$other = fn () => $this
    ->with([
        'alpha' => 'first',
    ])->label('x');

$total = collect([])->each(function () {
    run();
})->reduce([], function (A $carry): A {}) % (fn (): int => (((string) 100000 % 100) % compute((string) 1000000000 % 100)));
