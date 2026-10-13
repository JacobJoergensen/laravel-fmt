<?php

$result = DB::transaction(function () use (
    $provider,
    $socialUser,
    $submission,
    $accountGroup,
    $existingIdentity,
    $emailTrusted,
): void {
    doThing();
});
