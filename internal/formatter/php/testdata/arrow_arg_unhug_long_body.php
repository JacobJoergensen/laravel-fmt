<?php

class Foo
{
    public function bar(): void
    {
        if ($a) {
            $result = $items->map(fn (FooItem $model): BarItem => $this->baz($model, $scope, $lookupXxxxx[$model->id] ?? null));
            $result = $items->map(fn (FooItem $model): BarItem => $this->baz($model, $scope, $lookupXxxxxx[$model->id] ?? null));
            $query->whereHas('location.projects', static fn (Builder $q) => $q->whereIn('projects.id', Project::query()->visible()->select('id')));
        }

        return $this->items()
            ->whereHas('purchaseOrder', fn (Builder $query) => $query->whereIn('status', PurchaseOrderStatus::committedValues()))
            ->sum('amount');
    }
}
