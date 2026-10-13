<?php

$registry = new SourceRegistry()
    ->register(makeSource('alpha', [$first, $second]))
    ->register(makeSource('beta', [$third]))
    ->register(makeSource('gamma', [$fourth]));
