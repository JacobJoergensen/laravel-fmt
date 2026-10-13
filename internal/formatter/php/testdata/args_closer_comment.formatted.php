<?php

$rows = rowsFrom(
    "name,email\n"
        ."First,first@example.com\n"    // trailing its row
        ."Second,second@example.com\n", // trailing the last row
);

$total = sumOf(
    $first + 1,
    $second, // after the comma
);

$items = $query->where(
    'status',
    'open', // before the paren
)->get();

$none = emptyCall(/* nothing */);
