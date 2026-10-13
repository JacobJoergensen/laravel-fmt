<?php

// remove stale rows
$user->records()->delete();

// sync the pivot
$record->tags()
    ->attach($tagId, [
        'sort' => $order,
    ]);
