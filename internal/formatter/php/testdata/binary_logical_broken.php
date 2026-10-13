<?php

if ($account->verified_at !== null && $account->verified_at->gt(now()->subHours(self::WINDOW_HOURS))) {
    return;
}
$shouldDispatch = $worker->hasCapacity()
    && $worker->isHealthy()
    && $worker->isOnline()
    && $worker->lastHeartbeatAt()->gt(now()->subMinutes(5));
