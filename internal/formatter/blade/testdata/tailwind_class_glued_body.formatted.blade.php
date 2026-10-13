<div class="grid grid-cols-2 gap-3">
    <div class="rounded-lg p-3">
        <flux:text class="text-xs text-zinc-500">{{ __('common.orders') }}</flux:text
        ><flux:heading size="lg">{{ $stats['orders'] }}</flux:heading>
    </div>
    <div class="mt-1 truncate text-xs">
        @if ($match['section'])
            <span class="font-semibold text-zinc-500 uppercase">{{ $match['section'] }}</span>
        @endif
        {{ $match['before'] }}<span class="rounded-sm bg-amber-200 px-0.5">{{ $match['match'] }}</span
        >{{ $match['after'] }}
    </div>
    <p>{{ $a }}<span class="text-xs text-zinc-500">b</span>{{ $c }}</p>
    <span
        >{{ __('errors.trace_before') }}
        <code class="rounded border px-1.5 font-mono text-zinc-900">{{ $traceId }}</code>
        {{ __('errors.trace_after') }}</span>
</div>
