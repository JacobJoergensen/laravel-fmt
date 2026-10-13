{{ match ($repetitionType) {
    'daily' => $repetitionInterval === 1 ? 'day' : 'days',
    'weekly' => $repetitionInterval === 1 ? 'week' : 'weeks',
    'monthly' => $repetitionInterval === 1 ? 'month' : 'months',
    default => '',
} }}
