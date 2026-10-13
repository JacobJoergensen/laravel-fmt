<?php

class Limits
{
    public function resolve(?Agreement $agreement, ?Budget $budget): float
    {
        $cap = (float) (
            $agreement?->regular_hours_cap ?? $budget?->regular_hours_cap ?? self::DEFAULT_REGULAR_HOURS_CAP
        );

        $short = (float) ($agreement?->rate ?? 0);

        return (float) (
            $agreement?->overtime_hours_cap_for_the_first_tier
            ?? $budget?->overtime_hours_cap_for_the_first_tier
            ?? self::DEFAULT_CAP
        );
    }

    public function next(): int
    {
        return (int) ($this->schedule->strips()->where('scene_id', $this->sceneId)->max('split_index') ?? 1);
    }
}
