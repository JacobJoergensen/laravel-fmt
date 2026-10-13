<?php

class Totals
{
    public function sum(): void
    {
        $x
            & $rows->query->select($p1, $p2, $p3, $p4, $p5, $p6, $p7, $p8, $p9, $p10, $p11, $p12, $p13, $p14, $p15)
                ->get('id') % map('key', fn ($row) => [
                    $q1,
                    $q2,
                    $q3,
                    $q4,
                    $q5,
                    $q6,
                    $q7,
                    $q8,
                    $q9,
                    $q10,
                    $q11,
                    $q12,
                    $q13,
                    $q14,
                ])
                    ->sum(1);
    }
}
