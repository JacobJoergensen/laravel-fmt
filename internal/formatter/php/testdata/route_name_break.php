<?php

Route::get('/newsletters/track/{token}/pixel.gif', [EmailOpenTrackingController::class, 'pixel'])
    ->name('newsletters.tracking.pixel');
Route::post('/download', [PublicFileController::class, 'download'])->name('.download');
