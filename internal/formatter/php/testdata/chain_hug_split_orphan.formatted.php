<?php

class Registry
{
    public function collect($group, $owner): array
    {
        $needingLinkIdentifiers = $group
            ->filter(static fn (Entry $entry): bool => $entry->owner_id !== $owner->identifier)
            ->pluck('id')
            ->all();

        $model->forceFill([
            'first_key' => 'the first value',
            'second_key' => 'the second value',
            'third_key' => 'third',
        ])->save();

        return $needingLinkIdentifiers;
    }
}
