<div>
    <span @class([
        'flex size-7 items-center justify-center rounded-full border',
        'border-primary-600 bg-primary-50 text-primary-600' => $isCurrent,
    ])>
        <x-icon :name="$icon" />
    </span>
    <x-icon @class([
        'size-3 text-zinc-400 transition',
        'rotate-180' => $expanded,
    ]) />
    <flux:text
        @class([
            'text-[13px] font-medium',
            'text-text' => $isCurrent,
        ])
    >{{ $label }}</flux:text>
</div>
