<div>
    <div>
        <div>
            <div>
                <div>
                    <div>
                        <flux:badge size="sm" color="green" class="mt-3">{{ __('records.per_day', ['amount' => ($account?->currency ?? 'USD').' '.number_format($record->rate)]) }}
                        </flux:badge>
                        <flux:button wire:click="cancelEditingTheName" variant="ghost" size="sm">{{ __('common.cancel') }}
                        </flux:button>
                        <flux:button size="sm">{{ __('common.cancel') }}
                        </flux:button>
                        <span class="font-mono"><x-money :amount="(float) $ledger->opening_balance" :currency="$ledger->currency->value" /></span>
                        <span class="text-zinc-500">{{ __('records.a.really.long.translation.key.for.the.opening.balance.label.here') }}</span>
                        <flux:badge size="sm" color="{{ $record->type->color() }}">{{ __('enums.record_types.'.$record->type->value) }}
                            @if ($record->setting)
                                · {{ __('enums.record_settings.'.$record->setting->value) }}
                            @endif
                        </flux:badge>
                        <flux:badge size="sm">{{ $record->type->value }}
                            @if ($record->setting)
                                · {{ $record->setting->value }}
                            @endif
                        </flux:badge>
                        <div class="text-sm text-zinc-500">{{ __('records.per_day', ['amount' => ($account?->currency ?? 'USD').' '.number_format($record->rate)]) }}
                        </div>
                    </div>
                </div>
            </div>
        </div>
    </div>
</div>
