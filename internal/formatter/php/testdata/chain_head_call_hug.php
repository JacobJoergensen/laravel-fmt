<?php

$label = collect([
    $node->title,
    $node->subtitle,
    $node->category,
    $node->status,
])
    ->filter()
    ->implode(' / ');
