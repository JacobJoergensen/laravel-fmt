<div
    class="flex items-center justify-between gap-4 rounded-lg shadow-sm"
    @if ($editScope === 'this')
        style="display: none;"
    @endif
>
    {{ $slot }}
</div>
