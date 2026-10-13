<?php

$store->put(
    'this-cache-key-is-deliberately-long-enough-to-push-the-open-paren-line-past-the-print-width-on-its-own-and-then-some-more',
    [
        'ttl' => 3600,
        'tags' => ['subscribers', 'newsletter'],
    ],
);
