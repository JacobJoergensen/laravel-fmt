<?php

function syncSubscribers(array $subscribers): int
{
    if (blank($subscribers)) {
        return 0;
    }

    if (count($subscribers) > 100) {
        $this->warn('large batch');

        return 1;
    }

    $this->info('syncing');

    return 2;
}
