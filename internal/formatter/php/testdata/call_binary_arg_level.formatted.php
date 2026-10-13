<?php

$total = round(
    $parts['base'] * $unitPrice
    + $parts['extra'] * $unitPrice * $settings['extra_multiplier']
    + $parts['bonus'] * $unitPrice,
    2,
);

abort_unless(
    $user->membership !== null
    && $user->membership->group_id === $group->id
    && $user->isActiveMemberOfTheSelectedGroup(),
    403,
);

$sum = total(
    amount: $line->quantity * $line->unitPriceWithDiscountApplied
        + $line->shippingCostForThisLine
        + $line->handlingFeeForThisLine,
    currency: 'EUR',
);

$archive->addFromString(
    'readme.txt',
    "Export for the selected group\n"."This archive contains the requested records, in full detail.\n".'Generated: '
        .now(),
);
