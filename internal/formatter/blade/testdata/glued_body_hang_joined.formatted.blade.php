<div>
    <div>
        <flux:badge color="zinc" size="sm"
            >{{ $a }}{{ $b }} and some glued interior text that is long enough to overflow width</flux:badge>
        <flux:text class="mt-0.5 text-sm text-text-muted">
            {{ $profile->position }}{{ $profile->position && $profile->company ? ' @ ' : '' }}{{ $profile->company?->full_name }}
        </flux:text>
        <span
        >{{ $profile->position }}{{ $profile->position && $profile->company ? ' @ ' : '' }}{{ $profile->company?->full_name }} more</span>
        <flux:text class="mt-2">
            {{ __('forms.sign.signed_on') }} <x-datetime :value="$author->signed_at" format="datetime" />. and more words here to make it long
        </flux:text>
        <flux:button icon="bars-3" icon:variant="outline" size="sm" variant="filled">
            {{ $heading }} ({{ $count }}) and a long enough tail to overflow
        </flux:button>
        <span
            class="inline-flex items-center gap-1.5 rounded-full bg-emerald-50 px-2.5 py-1 text-xs font-medium text-emerald-700"
            ><span class="size-1.5 rounded-full bg-emerald-500"></span
            >{{ __('report.data_import.importable', ['count' => $stats['importable'] ?? 0]) }}</span>
        <a href="tel:{{ $phone }}" class="inline-flex items-center gap-1 text-blue-600"
            ><flux:icon.phone
                class="size-3.5"
            />{{ $warehouse->nearest_supplier_phone_number_formatted_for_display }}</a>
        <span class="x y"
            ><img
                src="/a.png"
                alt="a"
            >{{ $someVeryLongVariableNameThatKeepsGoingAndGoingAndGoingForWidthPurposes->withAnotherLongAccessor }}</span>
    </div>
</div>
