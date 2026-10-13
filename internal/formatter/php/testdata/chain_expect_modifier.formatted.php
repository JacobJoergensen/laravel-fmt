<?php

expect($response->getContent())->not->toContain(
    '"@context":"https://schema.org","@type":"BlogPosting","author":"JohnD"',
);
