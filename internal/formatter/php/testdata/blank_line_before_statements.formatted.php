<?php

function handle(array $rows): int
{
    foreach ($rows as $row) {
        $this->process($row);

        if (! $row->ok) {
            continue;
        }
    }

    if (empty($rows)) {
        throw new RuntimeException('empty');
    }

    if (count($rows) > 100) {
        $this->warn('large');
    }

    return count($rows);
}
