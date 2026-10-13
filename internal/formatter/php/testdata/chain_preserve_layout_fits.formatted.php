<?php

function recalc(array $lineIds): void
{
    InvoiceLineRow::query()
        ->whereIn('id', $lineIds)
        ->get()
        ->each(function (InvoiceLineRow $line): void {
            $line->recalculateBalance();
        });

    $rows = Report::query()
        ->where('status', 'open')
        ->get()
        ->map(fn ($row) => [
            'id' => $row->id,
            'total' => $row->total,
        ]);

    expect((float) $firstLine->fresh()->units)->toBe(25.0)
        ->and((float) $headLine->fresh()->units)->toBe(10.0)
        ->and((float) $tailLine->fresh()->units)->toBe(5.0);

    $collection->filter(fn ($item) => $item->isVisibleToTheCurrentUserAndNotArchived())
        ->map(fn ($item) => $item->presentForTheListView())
        ->values();

    Report::query()
        ->where(
            'a_really_long_column_name_that_goes_on_and_on',
            'some value that is also long enough to overflow the width',
        )
        ->get();

    expect($this->validator->violations($member))->toHaveCount(1)
        ->and($this->validator->violations($member)[0])
        ->toContain('below the hourly wage minimum for the role in team');
}
