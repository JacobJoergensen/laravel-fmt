<?php

$sql = <<<SQL
    select * from users
    where id = $userId
    SQL;

$raw = <<<'TEXT'
    Keep $this and \n verbatim
    TEXT;

function buildQuery(): string
{
    return <<<SQL
        select * from accounts
        SQL;
}

$total = $a + $b;
