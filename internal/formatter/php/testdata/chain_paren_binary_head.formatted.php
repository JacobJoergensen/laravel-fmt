<?php

final class Grouping
{
    public function grouped(Collection $items): Collection
    {
        return $items->sortBy(fn (Collection $rows, string $category): int => (
            StockCategory::tryFrom($category) ?? StockCategory::Other
        )->sortOrder());
    }

    public function short(?Category $a, Category $b): int
    {
        return ($a ?? $b)->sortOrder();
    }
}
