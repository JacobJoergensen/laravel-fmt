<div>
    <p class="text-xs">
        {{ $item->alpha_number_value }} ({{ (int) $item->current_amount_value }}
        → {{ $item->beta_value_text }}){{ ! $loop->last ? ', ' : '' }} and some more words here
    </p>
    <p class="text-xs">
        @foreach ($rows as $item)
            {{ $item->alpha_number_value }} ({{ (int) $item->current_amount_value }}
            → {{ $item->beta_value_text }}){{ ! $loop->last ? ', ' : '' }} and some more words here
        @endforeach
        . {{ __('alpha.beta.hint') }}
    </p>
    <h2 class="text-base">
        <x-icon.alpha class="mr-1 inline size-4" />
        {{ __('alpha.beta.gamma_delta') }} ({{ $this->pendingRowsCollection->count() }}) and some more words here
    </h2>
    <div class="text-xs">
        @if ($item->parent)
            {{ $item->parent->reference_number }} - {{ $item->parent->description_text }} · {{ $item->other }}
            more text after the echo run that pushes it over
        @endif
    </div>
    <p class="text-xs">
        @if ($item->parent)
            {{ $item->parent->reference_number }}
            - {{ $item->parent->description }}
        @endif
    </p>
</div>
