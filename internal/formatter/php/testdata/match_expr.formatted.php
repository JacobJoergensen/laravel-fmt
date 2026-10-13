<?php

$a = match ($x) {
    1 => 'one',
    2, 3 => 'few',
    default => 'many',
};
$b = match ($flag) {
    true => 'yes',
    false => 'no',
};
$c = match ($y) {};
