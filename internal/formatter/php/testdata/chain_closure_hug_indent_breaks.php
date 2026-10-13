<?php

$sub->where('visibility_type', VisibilityType::None->value)
    ->where(function ($i) use ($rangeStart, $rangeEnd, $extra): void {
        $i->foo();
    });
