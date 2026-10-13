<?php

$b = clone $a;
$c = clone $obj->inner;
$d = clone($a);
$e = clone($a, ['name' => 'Alice', 'age' => 30]);
$f = clone($a);
