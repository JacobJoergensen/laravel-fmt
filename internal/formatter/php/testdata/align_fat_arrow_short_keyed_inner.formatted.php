<?php

class BlogController
{
    public function index(): Response
    {
        return Inertia::render('Blog/Index', [
            'posts'         => $query->latest()->paginate(config('blog.results_per_page', 5)),
            'recentPosts'   => BlogPost::recent(3),
            'availableTags' => config('blog.tags'),
            'filters'       => ['search' => $search->toString(), 'tag' => $tag->toString() ?: 'all'],
        ]);
    }
}

class ImageAnalyzerService
{
    public function analyze(): array
    {
        return [
            'aspectRatio' => '16:9',
            'colors'      => [],
            'mood'        => ['brightness' => 'medium', 'temperature' => 'neutral'],
        ];
    }
}
