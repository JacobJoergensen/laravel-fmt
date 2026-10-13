<?php

Route::middleware(['auth'])->group(function () {
    Route::get('/billing/payment-method', function () {
        $tenantRecord = app(TenantScopeResolver::class)->require();

        return $tenantRecord->subscription()->redirectToUpdatePaymentMethod();
    })->middleware('permission:manage_billing')->name('billing.payment-method');
});

it('exports', function (): void {
    $this->get(route('accounts.dashboards.export.pdf', [
        'account' => $this->account->id,
        'dashboard' => $this->dashboard->id,
    ]))->assertRedirect()->assertSessionHas('exportPending');

    $rows = Report::query()
        ->where('status', 'open')
        ->whereIn('id', [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20])
        ->orderBy('name')
        ->get();

    Route::get('/x', function () {
        return 1;
    })
        ->middleware('permission:manage_integrations|manage_roles|manage_teams|manage_users')
        ->name('integrations.settings.overview');
});
