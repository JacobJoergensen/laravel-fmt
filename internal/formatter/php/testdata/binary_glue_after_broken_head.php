<?php

$total = summarize([
    'first' => 1,
    'second' => 2,
]) + $adjustment;

$total = summarize([
    'first' => 1,
    'second' => 2,
]) + $theRemainingAdjustmentForTheCurrentReportingPeriodOfTheLedgerThatIsFarTooLongToFitOnTheClosingLineHereAtAllReallyNow;

$total = summarize([
    'first' => 1,
    'second' => 2,
])->amountForTheCurrentReportingPeriodOfTheLedgerThatIsVeryLongIndeed() + $remainingAdjustmentForThePeriodOfTheLedgerAndMore;

$total = summarize([
    'first' => 1,
]) + adjustments([
    'second' => 2,
]);
