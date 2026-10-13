<div>
    <div class="mt-1 text-lg font-bold tabular-nums text-text-muted dark:text-zinc-100 flex items-center">{{ $item->value }}</div>
    <span class="mt-1 text-lg font-bold tabular-nums text-text-muted dark:text-zinc-100 flex items-center">{{ $item->name }}</span>
    <label class="mt-1 text-lg font-bold tabular-nums text-text-muted dark:text-zinc-100 flex items-center">{{ $label }}</label>
    <p>{{ number_format((float) $invoice->subtotal + (float) $invoice->tax - (float) $invoice->discount, 2) }}</p>
    <div class="short">ok</div>
</div>
