<?php

class Api
{
    /**
     * @return array{unit_price: float, base_limit: float, tier1_cap: float, tier2_cap: float, tier1_factor: float, bonus_factor: float}|null
     */
    public function rates(): ?array
    {
        return null;
    }

    /**
     * @phpstan-type Segment array{type: 'text', content: string}|array{type: 'link', link_kind: LinkKind, identifier: string, label: string}
     */
    public function segment(): void {}

    /**
     * @param ?array{section: string, region: string, time_of_day: string, item_number: string, is_flagged: bool, is_hidden: bool} $heading
     */
    public function heading(?array $heading): void {}
}
