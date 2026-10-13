<?php

$result = $repository->findActiveSubscribers()
    ->withProfileAndAvatar()
    ->withRecentActivity()
    ->orderByLastSeenAt()
    ->limit(50);
