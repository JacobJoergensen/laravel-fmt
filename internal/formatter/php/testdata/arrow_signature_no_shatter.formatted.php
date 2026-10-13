<?php

class Report
{
    public function compute(): void
    {
        $this->elapsed_estimate_seconds = (int) $items
            ->filter(static fn (ItemLog $item): bool => $item->status->isShippable())
            ->sum('duration_seconds');
    }
}
