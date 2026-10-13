<?php

function reportSql(int $id): string
{
    return <<<SQL
        select *
            from orders
        where id = $id
        SQL;
}
