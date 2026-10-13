<?php

$color = match ($status) {
    self::Active => 'green', // active items
    self::Paused => 'amber', // paused items
    default => 'gray', // everything else
};
