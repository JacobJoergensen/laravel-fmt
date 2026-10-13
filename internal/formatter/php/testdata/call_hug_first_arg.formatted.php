<?php

$writer->writeRow([
    $record->id,
    $record->name,
    $record->category?->label,
    $record->isActive ? 'Yes' : 'No',
    $record->notes,
], null, 'B'.$line);
