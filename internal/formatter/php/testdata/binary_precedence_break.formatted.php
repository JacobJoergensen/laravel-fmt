<?php

class Report
{
    public function render(): array
    {
        if (
            $claim->incentive_type->isSoldAtDiscountForTheWholeClaimPeriod()
            && $claim->estimated_discount_percentage_value_for_display !== null
        ) {
            return [];
        }

        if (
            $this->query->where('a', 1)
                ->where('b', 2)
                ->where('c', 3)
                ->where('d', 4)
                ->where('e', 5)
                ->where('f', 6)
                ->exists() && $this->enabled
        ) {
            return [];
        }

        return [
            'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-400 font-medium' =>
                $currentStatus === $status && $status === Status::WorkingFromHome,
            'opacity-60' => $currentStatus === $status && $status === Status::OutSick,
        ];
    }
}
