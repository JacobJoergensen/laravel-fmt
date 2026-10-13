<button wire:click="setStatus({{ $row->id }}, {{ $active ? "'on'" : 'off' }})" class="btn">Toggle</button>
