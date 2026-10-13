<?php

$aspect = match (true) {
    abs($ratio - 2.39) < 0.1 => '2.39:1',
    abs($ratio - 2.35) < 0.1 => '2.35:1',
    abs($ratio - 1.78) < 0.05 => '16:9',
    abs($ratio - 1.00) < 0.05 => '1:1',
    default => 'unknown',
};
