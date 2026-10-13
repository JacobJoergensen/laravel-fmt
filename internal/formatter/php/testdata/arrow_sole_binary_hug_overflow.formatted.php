<?php

function a()
{
    if ($x) {
        if ($y) {
            $item = $order->lines->first(
                static fn (OrderLine $line): bool => $line->status->isReadyForShippingToTheCustomer()
                    && $line->packed_at !== null,
            );
        }
    }
    $item = $order->lines->first(static fn (OrderLine $line): bool => $line->status->isReadyForShippingToTheCustomer()
        && $line->packed_at !== null);
}
