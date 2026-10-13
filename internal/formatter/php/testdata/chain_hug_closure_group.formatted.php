<?php

Route::prefix('blog')->name('blog')->group(function (): void {
    Route::get('/', [BlogController::class, 'index']);
});
