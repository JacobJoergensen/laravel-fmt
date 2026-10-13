<?php

enum Status: string implements HasLabel
{
    case Active = 'active';
    case Pending = 'pending';
    case Closed = 'closed';

    const DEFAULT = self::Active;

    public function label(): string
    {
        return match ($this) {
            self::Active => 'Active',
            self::Pending => 'Pending',
            self::Closed => 'Closed',
        };
    }
}
