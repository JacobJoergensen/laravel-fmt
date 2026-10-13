<div class="grid grid-cols-2 gap-3">
    <div class="rounded-lg p-3">
        <flux:text class="text-zinc-500 text-xs">{{ __('common.orders') }}</flux:text
        ><flux:heading size="lg">{{ $stats['orders'] }}</flux:heading>
    </div>
    <div class="mt-1 truncate text-xs">
        @if ($match['section'])
            <span class="text-zinc-500 font-semibold uppercase">{{ $match['section'] }}</span>
        @endif
        {{ $match['before'] }}<span class="px-0.5 bg-amber-200 rounded-sm">{{ $match['match'] }}</span>{{ $match['after'] }}
    </div>
    <p>{{ $a }}<span class="text-zinc-500 text-xs">b</span>{{ $c }}</p>
    <span>{{ __('errors.trace_before') }}
        <code class="text-zinc-900 rounded border px-1.5 font-mono">{{ $traceId }}</code>
        {{ __('errors.trace_after') }}</span>
</div>
