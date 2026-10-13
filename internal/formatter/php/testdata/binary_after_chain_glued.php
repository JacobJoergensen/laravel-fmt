<?php

class Report
{
    public function purge(): bool
    {
        return TaskEntryRecord::query()->where('warehouse_identifier', $warehouseId)->where('ship_date_identifier', $shipDateId)->delete() > 0;
    }

    public function ids(): Collection
    {
        return $order->orderComponents->map(fn (Component $c): int => $c->identifier)->filter()->unique()->values() ?? collect();
    }

    public function either(): string
    {
        return $this->firstCandidateValue.$this->secondCandidateValue.$this->thirdCandidateValue.$this->fourthCandidate;
    }

    public function tails(object $row): void
    {
        $next = (int) TaskEntryRecord::query()->where('warehouse_identifier', $row->warehouse_identifier)->max('sort') + 1;
        $this->dayValueList = $row->recurrence_days?->map(fn (Weekday $day): string => $day->value)->values()->all() ?? [];
        $last = (int) ($this->schedule->entries()->where('warehouse_identifier', $row->warehouse_identifier)->max('idx') ?? 1);
        $single = $this->resolveTheWarehouseIdentifierForThisRecord($row->warehouse_identifier, $row->ship_date) ?? $row;
        $callTail = TaskEntryRecord::query()->where('warehouse_identifier', $row->warehouse_identifier)->max('sort') + $this->offset($row);
    }

    public function prefixed(int $ownerId): array
    {
        return [
            'next' => (int) TaskEntryRecord::query()->where('warehouse_identifier', $ownerId)->where('ship_date_identifier', $ownerId)->max('sort_order') + 1,
            'last' => (int) TaskEntryRecord::query()->where('warehouse_identifier', $ownerId)->where('ship_date_identifier', $ownerId)->value('sort_order') ?? 1,
            'none' => ! TaskEntryRecord::query()->where('warehouse_identifier', $ownerId)->where('ship_date_identifier', $ownerId)->where('kind', 'x')->exists() && $ownerId > 0,
        ];
    }
}

function anchorMeasurementAvailable(): bool
{
    static $available = null;

    return $available ??= app(AnchorMeasurer::class)->measure('<p>{{probe}}</p>', ['{{probe}}' => FieldType::Signature]) !== [];
}
