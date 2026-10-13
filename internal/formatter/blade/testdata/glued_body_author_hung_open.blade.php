<div class="space-y-4">
    <flux:field
        ><flux:label>{{ __('common.name') }}</flux:label
        ><flux:input wire:model="labelName" :placeholder="__('contacts.label_placeholder')" /><flux:error
            name="labelName"
    /></flux:field>
    <flux:field><flux:label>{{ __('common.color') }}</flux:label
        ><flux:input type="color" wire:model="labelColor"
    /></flux:field>
</div>
