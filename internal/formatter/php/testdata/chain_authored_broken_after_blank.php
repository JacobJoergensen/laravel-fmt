<?php

$now = now();

$group->members()
    ->syncWithoutDetaching([
        $user->id => [
            'role' => $role->value,
            'added_at' => $now,
        ],
    ]);
