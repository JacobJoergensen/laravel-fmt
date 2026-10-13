<div>
    <span
        @class([
            'rounded-full px-2 py-1 text-xs font-medium',
            'bg-emerald-100 text-emerald-800' => $isActive,
            'bg-zinc-100 text-zinc-600' => ! $isActive,
        ])
    >badge</span>
    <span @class(['a', 'b' => $c])>short stays flat</span>
</div>
