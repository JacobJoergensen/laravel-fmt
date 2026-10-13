<?php

return Attribute::get(fn (): string => match ($status) {
    'open' => 'Open',
    'closed' => 'Closed',
    default => 'Unknown',
});
