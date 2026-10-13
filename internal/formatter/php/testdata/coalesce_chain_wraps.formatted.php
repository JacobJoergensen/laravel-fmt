<?php

$timezone = $userAccountSettings->preference
    ?? $installationDefault->timezone
    ?? $systemFallbackTimezone
    ?? $hardcodedUtcDefault;
