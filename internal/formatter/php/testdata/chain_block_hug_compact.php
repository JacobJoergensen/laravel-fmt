<?php

it('charges', function (): void {
    $line = InvoiceLineRow::factory()->forAccount($account)->create([
        'is_discounted' => false,
        'markup_rate' => 25,
    ]);

    $this->get(route('links.public.media', [
        'token' => $this->share->token,
        'media' => $other->getFirstMedia('file'),
    ]))->assertNotFound();

    $rows = Report::query()->where('status', 'open')->whereIn('id', [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20])->orderBy('name')->get();

    Route::prefix('admin')->middleware('auth')->group(function () {
        require base_path('routes/admin.php');
    });

    $line = InvoiceLineRow::factory()->forAccountWithAVeryLongMethodNameThatPushesTheOpenerPastTheWidthLimit($account)->create([
        'is_discounted' => false,
    ]);

    Route::get('/newsletters/track/{token}/pixel.gif', [EmailOpenTrackingController::class, 'pixel'])->name('newsletters.tracking.pixel');
});
