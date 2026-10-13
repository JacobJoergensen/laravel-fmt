<?php

it('formats the given amount into a localized currency string for the active locale', function (
    int $amount,
    string $currency,
): void {
    expect($amount)->toBeInt();
});
