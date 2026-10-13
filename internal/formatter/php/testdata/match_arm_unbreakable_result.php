<?php

$reason = match (true) {
    $market === Market::Western && $type === ContractKind::Cancelation => 'Western rules require a handwritten signature to cancel a longer-term service agreement, and electronic form is excluded.',
    $market === Market::Western && $type === ContractKind::Withdrawal => 'A withdrawal from a signed agreement needs a written notice handed to the supplier, so treat it as paper for now.',
    default => 'Online checkout is currently unavailable for the basket.',
};
