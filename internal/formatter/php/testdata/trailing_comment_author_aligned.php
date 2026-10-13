<?php

$shape = match ($code) {
    '10' => Shape::Circle,     // 10 Round
    '11' => Shape::Rectangle,  // 11 Four sides
    '12' => Shape::Star,       // 12 Pointed
};

$limits = [
    'small' => 10,    // per page
    'medium' => 250,  // per export
    'large' => 5000,  // per import
];

$first = 1;     // aligned by the author
$second = 22;   // with the line above

$ragged = [
    'a' => 1,    // not aligned
    'bb' => 2, // by the author
];

$pairs = [
    'a' => 1,  // first group
    'b' => 2,  // first group
    'ccc' => 3,    // second group
    'ddd' => 4,    // second group
];
