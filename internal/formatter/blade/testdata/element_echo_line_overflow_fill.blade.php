<div>
    <div>
        <div>
            <div>
                <div>
                    <div class="mt-1 text-xs text-zinc-500">
                        {{ $record->parent->currency }} {{ number_format((float) $record->parent->grand_total_estimated) }}
                    </div>
                    <div class="mt-1 text-xs text-zinc-500">
                        {{ $record->parent?->currency ?? $owner?->currency ?? 'USD' }} {{ number_format((float) $record->amount, 2) }}
                        second authored line stays
                    </div>
                    <div class="mt-1 text-xs text-zinc-500">
                        {{ $record->parent->currency }} {{ number_format((float) $record->total) }}
                    </div>
                    <div class="mt-1 text-xs text-zinc-500">
                        Total amount for the selected period and the current owner of this record is: {{ trans('alpha.beta', [
                            'first' => $record->first,
                            'second' => $record->second,
                        ]) }} after
                    </div>
                </div>
            </div>
        </div>
    </div>
</div>
