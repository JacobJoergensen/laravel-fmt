<?php

return Inertia::render('Blog/Index', [
    'posts' => $query->latest()->paginate(5),
    'recentPosts' => BlogPost::recent(3),
]);
