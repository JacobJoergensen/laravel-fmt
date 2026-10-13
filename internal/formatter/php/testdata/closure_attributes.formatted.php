<?php

$f = #[A] fn () => 1;
$g = #[A, B(1)] function () {
    return 1;
};
$h = #[A] #[B] static fn ($x) => $x;
array_map(#[Pure] fn ($x) => $x, []);
$i = #[A] /* c */ fn () => 2;
