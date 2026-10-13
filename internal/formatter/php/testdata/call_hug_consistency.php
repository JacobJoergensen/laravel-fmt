<?php

class Report
{
    public function widgets(string $query): array
    {
        return Widget::query()
            ->whereKeyNot($this->ownerId)
            ->when($query !== '', static function (Builder $q) use ($query): void {
                $q->whereLike('name', $query);
            })
            ->when([$this->firstColumnName, $this->secondColumnName, $this->thirdColumnName, $this->fourth], function (Builder $q): void {
                $q->latest();
            })
            ->get();
    }

    protected function code(): Attribute
    {
        return Attribute::get(fn (): string => strtoupper(substr((string) $this->primary_label, 0, 2).substr((string) $this->second_label, 0, 2)));
    }

    protected function label(): Attribute
    {
        return Attribute::get(fn (): string => strtoupper(substr((string) $this->primary_label, 0, 2).substr((string) $this->tag, 0, 2)));
    }
}
