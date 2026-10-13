<div>
    <div>
        @if ($items->isNotEmpty())
            <span class="item-subject">
                [{{ $items->pluck('display_name')->implode(', ') }}] and a little more text</span>
        @endif
        <a
            href="tel:{{ $record->phone }}"
            class="mt-1 inline-flex items-center gap-1.5 font-mono text-sm font-semibold text-red-800"
        >
            <flux:icon.phone class="size-3.5" />{{ $record->phone }}</a>
        @foreach ($options as $option)
            <flux:select.option value="{{ $option->value }}">
                {{ __('enums.options.'.$option->value) }} ({{ $option->value }}) extra</flux:select.option>
        @endforeach
        <span class="text-text-muted">
            +{{ $record->assignees->count() - 1 }} {{ __('tasks.more_assignees_label_text') }}</span>
    </div>
</div>
