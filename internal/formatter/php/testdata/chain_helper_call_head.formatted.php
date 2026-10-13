<?php

Route::get(
    '/manifest.json',
    fn () => response()->json([
        'name' => 'Notes',
        'short_name' => 'Notes',
        'description' => 'Small inventory management',
    ])
        ->header('Content-Type', 'application/manifest+json')
        ->header('Cache-Control', 'public, max-age=3600'),
)->name('manifest');

if ($available === 0) {
    test()->markTestSkipped(
        "0/{$total} sample fixtures available. Add the source samples to tests/Fixtures/Samples to enable these tests.",
    );
}

expect($visibleIdentifiersForTheCurrentUser)->toContain($firstVisibleEvent->id)
    ->not->toContain($strangerEventForAnotherTeam->id);

$rows = Report::query()->where('status', 'open')->where('owner_id', $ownerId)->orderBy('created_at')->limit(50)->get();

$thing = new ReportBuilder($first)->where('status', 'open')->where('owner_id', $ownerId)->orderBy('created_at')->get();

$rows = Report::query()
    ->where('status', 'open')
    ->where('owner_id', $ownerIdentifierForTheCurrentTenant)
    ->orderBy('created_at')
    ->limit(50)
    ->get();
