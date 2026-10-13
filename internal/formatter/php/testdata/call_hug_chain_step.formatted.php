<?php

User::query()
    ->where('active', true)
    ->orderBy('name')
    ->paginate(10, [
        'columns' => ['id', 'name', 'email'],
    ]);
