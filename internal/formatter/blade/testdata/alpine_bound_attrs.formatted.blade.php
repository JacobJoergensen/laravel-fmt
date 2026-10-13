<div>
    <div :style="'order: ' + (checks.{{ $key }} ? {{ $loop->index }} : 0)">a</div>
    <div :class="{ 'opacity-50': dragging === {{ $panel->id }} }">b</div>
    <flux:badge :color="$source === 'image' ? 'pink' : 'cyan'">c</flux:badge>
    <flux:button wire:confirm="Transfer ownership? You'll lose owner authority.">d</flux:button>
    <flux:button wire:click="save({{ $id }})">e</flux:button>
</div>
