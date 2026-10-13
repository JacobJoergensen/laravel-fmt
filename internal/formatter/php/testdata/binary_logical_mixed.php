<?php

$eligible = $member->hasActiveSubscription()
    && $member->completedOnboarding()
    && $member->withinUsageQuota()
    && $member->emailVerifiedRecently()
    && $member->acceptedLatestTerms();
