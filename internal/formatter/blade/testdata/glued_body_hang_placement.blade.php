<div>
    <div>
        <div>
            <div>
                <span>{{ __('event-pages.opening') }}: <x-datetime :value="$eventPage->opening" :tz="$epTz" :on-date="$eventPage->date" format="time" /></span>
                <span>{{ __('event-pages.closes') }}: <x-datetime :value="$eventPage->closes" :tz="$epTimeZone" :on-date="$eventPage->date" format="time" /></span>
                <flux:button wire:click="$set('photoFilter', '')" size="xs" :variant="$photoFilter === '' ? 'primary' : 'ghost'">{{ __('common.all') }} ({{ $property->getMedia('photos')->count() }}) </flux:button>
                <span class="inline-flex items-center gap-1.5 rounded-full bg-amber-50 px-2.5 py-1 text-xs font-medium text-amber-700" title="x">{{ __('report.data_import.credits_skipped_with_a_long_label', ['count' => $stats['credit'] ?? 0]) }}</span>
                <span class="inline-flex items-center gap-1.5 rounded-full bg-amber-50 px-2.5 py-1 text-xs font-medium text-amber-700" title="y">{{ $first }}{{ $second }}</span>
                <span class="inline-flex items-center gap-1.5 rounded-full bg-amber-50 px-2.5 py-1 text-xs font-medium text-amber-700"><span class="size-1.5 rounded-full bg-amber-500"></span>{{ __('report.data_import.credits_skipped', ['count' => $stats['credit'] ?? 0]) }}</span>
            </div>
        </div>
    </div>
</div>
