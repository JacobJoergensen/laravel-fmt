<?php

$value = match ($mode) {
    Mode::FixedCount => $builder->take($limit),
    Mode::UntilDate  => $cutoff !== null ? $builder->until($cutoff->copy()->endOfDay()) : null,
    Mode::Forever    => null,
};
