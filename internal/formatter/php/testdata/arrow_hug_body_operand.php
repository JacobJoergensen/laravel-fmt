<?php

function pick($items, $matched, $userId)
{
    if ($items) {
        $next = $items->first(fn (FooBarBazQuxQuuxCorgeGrault $t): bool => (float) $t->max_amount > (float) $matched->max_amount && $t->owner_id !== $userId);
    }

    return run('ids', static fn (Builder $q) => $q->orderByRaw('CASE WHEN id IN (1, 2, 3, 4, 5, 6, 7, 8, 9, 10) THEN 0 ELSE 1 END AND MORE'));
}
