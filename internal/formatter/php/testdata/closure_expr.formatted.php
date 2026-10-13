<?php

$f = function () {};
$g = function ($x) {
    return $x;
};
$h = function ($x, $y) {
    return $x + $y;
};
$i = function ($x) use ($y) {
    return $x + $y;
};
$j = function ($x) use (&$y, $z) {
    return $x;
};
$k = function ($x): int {
    return $x;
};
$l = static function () {
    return 1;
};
