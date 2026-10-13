<?php

class Panel
{
    public function classes(): array
    {
        return [
            'alpha-one beta-two gamma-three delta-four epsilon-five zeta-six eta-seven' => $item['state'] === 'archived',
            'theta' => $this->firstLongPropertyName !== null && $this->secondLongPropertyName !== null && $this->thirdLongPropertyName,
            'iota' => 'prefix text that is long enough to overflow the line '.$this->label.' and the suffix text that follows',
            'kappa' => $item['state'] === 'draft',
        ];
    }

    public function nested(): array
    {
        return $this->merge('panel', ['alpha-one beta-two gamma-three delta-four epsilon-five zeta-six' => $value ?? $default]);
    }
}
