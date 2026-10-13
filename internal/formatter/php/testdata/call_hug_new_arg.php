<?php

Notification::send($recipients, new SubscriptionExpired(
    $account->id,
    $account->plan_name,
    $account->expires_at?->format('M j, Y'),
));
