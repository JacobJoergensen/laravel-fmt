<?php

function build(): void
{
    $row = [
        'description' => sprintf(
            'Row %d - %s - %s',
            $report->rowNumber,
            $member->fullDisplayName,
            $this->formatItemsSummaries($items, $defaultCap, $extendedCap),
        ),
    ];
}
