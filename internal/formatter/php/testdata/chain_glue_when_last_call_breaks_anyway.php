<?php

class Planner
{
    private function categoriesByEntry(Collection $days): Collection
    {
        return $this->scheduledEntriesForTheWholeProductionWindow($days)->mapWithKeys(fn (Entry $entry): array => [
            $entry->id => $entry->name,
        ]);
    }

    public function run(Ledger $ledger, int $completedDays, int $remainingDays): void
    {
        $ledger->lineItems()->lazyById()->each(function (LineItem $line) use ($completedDays, $remainingDays, &$computed, &$skippedManual, &$skippedNoData): void {
            $line->save();
        });
    }

    public function render(): View
    {
        return $this->view()->title(__('common.day_number', ['number' => $this->shootDay->day_number]).' · '.__('nav.script_supervisor_overview'));
    }

    public function ladder(): Collection
    {
        return $this->entries()->filter(fn (Entry $entry): bool => $entry->active)->map(fn (Entry $entry): string => $entry->name);
    }
}
