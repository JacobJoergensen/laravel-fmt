<?php

$pixels[] = [
    'r' => ($rgb >> 16) & 0xFF,
    'g' => ($rgb >> 8) & 0xFF,
    'b' => $rgb & 0xFF,
];

$collapsed_no_comma = [
    'r' => 1,
    'g' => 2,
    'b' => 3,
];

$stays_flat_with_comma = [1, 2, 3];
