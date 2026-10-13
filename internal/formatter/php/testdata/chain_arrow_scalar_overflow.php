<?php

$q->where('contacts.created_by', $user->id)
    ->orWhereHas(
        'projects.users',
        static fn (\Illuminate\Database\Eloquent\Builder $u) => $u->where('users.id', $user->id),
    );
