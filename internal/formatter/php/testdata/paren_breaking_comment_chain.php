<?php

class Report
{
    public function totals(): void
    {
        $total = (collect()->sum(outstandingBalanceIncludingRefundsAdjustmentsCreditsAndWriteOffsForEveryWarehouseAndShipDate()) // net
        );
        $count = (TaskEntryRecord::query()->count() // all rows
        );
    }
}
