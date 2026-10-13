<?php

[$a, , $c] = $values;
[, $second] = $pair;
foreach ($rows as [$id, , $name]) {
    record($id, $name);
}
