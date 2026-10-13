<?php

class Totals
{
    public function sum(): void
    {
        $total = $base
            + map(static fn ($item) => $prefix
                .$this->label($item->name, [$v1, $v2, $v3, $v4, $v5, $v6, $v7, $v8, $v9, $v10]))
                ->sum($scale);
    }
}
