<?php

Route::get('/', fn () => Inertia::render('Home', [
    'recentPosts' => BlogPost::recent(3),
]))->name('home');
$posts->each(fn ($post) => $entries->push([
    'loc' => $post->slug,
    'priority' => '0.8',
]));
