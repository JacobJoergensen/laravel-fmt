<div>
    <button
        type="button"
        @class([
            'btn',
            'btn-active' => $isActive,
            'btn-disabled' => $disabled,
        ])
        wire:click="save"
    >
        Save
    </button>
</div>
