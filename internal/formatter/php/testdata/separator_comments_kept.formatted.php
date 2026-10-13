<?php

f($a /* c1 */, $b);
f($a // c2
, $b);
$x = [1 /* c3 */, 2];
$y = ['k' => 1 /* c4 */, 'j' => 2];
function g($p /* c5 */, $q) {}

echo $a /* c6 */, $b;
use A\{B /* c7 */, C};

global $g1 /* c8 */, $g2;
unset($u1 /* c9 */, $u2);
isset($i1 /* c10 */, $i2);
new Foo($n1 /* c11 */, $n2);
[$l1 /* c12 */, $l2] = $arr;
$m = match ($v) { 1 /* c13 */, 2 => 'x', default => 'y' };
fn($p1 /* c14 */, $p2) => 1;
static $s1 /* c15 */, $s2;
const K1 = 1 /* c16 */, K2 = 2;
for ($i = 0 /* c17 */, $j = 0; $i < 1; $i++) {}
class Z
{
    use T1 /* c18 */, T2;

    public $pa /* c19 */, $pb;
}

function h(): A /* c20 */|B {}

$z = [1, 2 /* c21 */, ];
f($a, /* c22 */ $b);
$formatted = [1, 2];
use A\B /* x */, A\C;

$cast = (/* a */int) $v;
