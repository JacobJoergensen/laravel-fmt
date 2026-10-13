<div>
    <div>
        <div>
            <div>
                <flux:button
                    variant="primary"
                    wire:click="preview"
                    icon:trailing="arrow-right"
                >{{ __('report.data_import.preview') }}</flux:button>
                <flux:button
                    variant="primary"
                    wire:click="preview"
                    icon:trailing="arrow-right"
                >{{ __('report.data_import.preview') }}</flux:button>
                <flux:heading size="lg">
                    {{ __('some.really.long.translation.key.name.that.keeps.going.and.going.for.width') }}
                </flux:heading>
                <flux:heading>
                    {{ __('some.really.long.translation.key.name.that.keeps.going.and.going.and.going.for.width') }}
                </flux:heading>
                <flux:button variant="primary" size="sm">{{ __('common.save') }}</flux:button>
                <div class="p-4" id="x">
                    {{ __('some.really.long.translation.key.name.that.keeps.going.and.going.for.width') }}
                </div>
                <span
                    class="text-sm font-medium"
                    data-x="1"
                ><x-money :amount="$line->amount_in_cents_for_display" :currency="$wallet->currency" /></span>
                <a
                    href="#"
                    class="underline"
                >{{ __('some.really.long.translation.key.name.that.keeps.going.and.going.for.width') }} </a>
                <flux:button variant="primary" size="sm">
                    {{ __('some.really.long.translation.key.name.that.keeps.going.and.going.for.width') }}
                </flux:button>
            </div>
        </div>
    </div>
</div>
