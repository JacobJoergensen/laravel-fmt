<?php

$f = fn () => null;
$g = fn ($x) => $x + 1;
$h = fn ($x, $y) => $x + $y;
$i = fn ($x): int => $x;
$j = static fn ($x) => $x;
