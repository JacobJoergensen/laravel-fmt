<?php

$csv = collect([
    $node->titleValue,
    $node->subtitleValue,
    $node->categoryValue,
    $node->statusValue,
    $node->extraValue,
    $node->finalValue,
])->implode(', ');
