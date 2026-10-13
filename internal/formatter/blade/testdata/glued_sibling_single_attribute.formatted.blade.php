<div class="rounded-lg p-4 text-sm">
    <div class="flex items-baseline justify-between">
        <span class="text-text-subtle">{{ __('enums.member_types.performer') }}</span
        ><span
            class="font-mono font-semibold text-text tabular-nums"
        >{{ $this->visibleProductionCrewMembers->count() }}</span>
    </div>
    <span class="block text-[11px] tracking-wide text-zinc-500 uppercase">{{ __('common.call_time_label') }}</span>
    <span class="block text-[11px] tracking-wide text-zinc-500 uppercase">{{ __('common.call_time_label_long') }}:</span>
</div>
