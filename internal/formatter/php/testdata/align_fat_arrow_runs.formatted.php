<?php

class Rules
{
    public function reason(OrderType $type, Market $market): ?string
    {
        return match (true) {
            $market === Market::North && $type === OrderType::ExtendedWarrantyCancellationRequest => __(
                'rules.market.north_cancel',
            ),
            $market === Market::North && $type === OrderType::Exchange => __('rules.market.north_exchange'),
            default                                                    => __('rules.market.not_permitted'),
        };
    }

    public function events(): array
    {
        return match ($this) {
            self::Calendar => ['created', 'updated', 'deleted', 'restored', 'budget_exported', 'report_exported'],

            self::LongerNameForTheSupervisor => ['entry_deleted', 'report_locked'],
        };
    }

    public function labels(): array
    {
        return [
            'no_coordinates_found' => 'No coordinates',

            'compass' => ['N', 'NNE', 'NE', 'ENE', 'E', 'ESE', 'SE', 'SSE', 'S', 'SSW', 'SW', 'WSW', 'W', 'WNW'],
        ];
    }

    public function directions(): array
    {
        return [
            'no_coordinates_found' => 'No coordinates',
            // compass points
            'compass' => ['N', 'NNE', 'NE', 'ENE', 'E', 'ESE', 'SE', 'SSE', 'S', 'SSW', 'SW', 'WSW', 'W', 'WNW'],
        ];
    }

    public function schema(): array
    {
        return [
            'setup'      => $schema->integer()->min(1)->required(),
            'angle_name' => $schema->string()->enum($this->values(Angle::cases()))->required()->nullable(),
            'movement'   => [
                'a' => 1,
            ],
            'estimated_duration_with_a_long_key_name' => $schema->integer(),
        ];
    }
}
