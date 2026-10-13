<?php

$transform = fn (array $values): array => $values
    |> array_filter(...)
    |> (fn ($v) => array_map($mapper, $v))
    |> array_values(...);
