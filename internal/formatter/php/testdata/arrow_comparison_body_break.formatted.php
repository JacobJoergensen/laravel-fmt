<?php

$selected = $collection->first(
    static fn (Record $record): bool => $record->fetchMetadataValue('classification_key')
        === ReportCategory::Approved->value,
);
