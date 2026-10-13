<?php

$role = $user->role ?? 'guest';
$locale = $request->input('locale') ?? config('app.locale') ?? 'en';
$bio = $user?->profile?->bio ?? 'No bio provided';
$options['timeout'] ??= 30;
$preferredDisplayTimezone = $userAccountSettings->resolvedTimezonePreferenceValue
    ?? $installationWideDefaultTimezoneConfigurationSetting;
