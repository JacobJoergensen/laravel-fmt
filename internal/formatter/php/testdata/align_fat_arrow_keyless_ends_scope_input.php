<?php

$spread = [
    'aa' => 1,
    'bbbbbb' => 2,
    ...$extra,
    'cc' => 3,
];

$bareValue = [
    'aa' => 1,
    $other,
    'bbbbbb' => 2,
    'c' => 3,
];

$keylessArray = [
    'aa' => 1,
    ['x' => 1],
    'bbbbbb' => 2,
    'c' => 3,
];

$leading = [
    ...$extra,
    'aa' => 1,
    'bbbbbb' => 2,
];

$trailing = [
    'aa' => 1,
    'bbbbbb' => 2,
    ...$extra,
];

$consecutive = [
    'aa' => 1,
    ...$extra,
    $other,
    'bbbbbb' => 2,
    'c' => 3,
];
