<?php

if ($a) {
    $x = 1;
} elseif ($b) {
    $x = 2;
} else {
    $x = 3;
}
while ($a) {
    break;
}
do {
    $i = $i + 1;
} while ($i < 10);
for ($i = 0; $i < 10; $i = $i + 1) {
    echo $i;
}
foreach ($items as $value) {
    echo $value;
}
switch ($x) {
    case 1:
        $y = 'one';
        break;
    default:
        $y = 'other';
}
try {
    doRisky();
} catch (Exception $e) {
    handle();
} finally {
    cleanup();
}
