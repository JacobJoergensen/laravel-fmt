<?php

use Illuminate\Validation\Rules\Enum;

$rule = new Enum(Status::class);
$name = Enum::class;
$is = $value instanceof Enum;

enum Suit: string implements HasColor
{
    case Hearts = 'h';
}
