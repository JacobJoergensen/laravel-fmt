<?php

return Attribute::get(fn (): float => abs(
    (float) $this->entries->where('kind', LedgerKind::Debit)
        ->filter(fn (LedgerEntry $e): bool => $e->state->isPosted())
        ->sum('value'),
));
