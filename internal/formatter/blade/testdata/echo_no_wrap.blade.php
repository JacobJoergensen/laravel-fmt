<p>{{ number_format((float) $invoice->subtotal + (float) $invoice->tax + (float) $invoice->shipping - (float) $invoice->discount, 2) }}</p>
<p>{{ collect([$a, $b])->filter()->join(' · ') }}</p>
