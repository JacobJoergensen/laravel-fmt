<?php

final class Exporter
{
    public function handle($key, $request, $account): void
    {
        throw_if(
            RateLimiter::tooManyAttempts($key, self::HOURLY_LIMIT),
            RuntimeException::class,
            __('auth.registration.too_many'),
        );

        $token = GenerateReportPdfJob::enqueue(
            new InvoicePdfDocument($account, $statement, $this->redactsUserContacts($account)),
            $request->user()->id,
        );

        $response = Http::withToken($accessToken)
            ->get("https://graph.example.com/v1.0/drives/{$driveId}/items/{$externalId}/details", [
                '$select' => 'name,size,file,webUrl',
            ]);

        $this->domain = app(TenantScopeResolver::class)->run($tenantRecord, fn () => DomainRecord::factory()->create([
            'hostname' => 'client.test',
        ]));

        $this->domain = app(TenantScopeResolver::class)
            ->run($tenantRecord, fn () => AccountDomainAliasRecord::factory()->create([
                'hostname' => 'client.test',
            ]));

        $outsideAccount = collect($service->search($this->user->fresh(), 'monthly report'))
            ->firstWhere('type', ResultKind::Action)['hits'] ?? [];

        $user = User::query()->create([
            'name' => $request->input('name'),
        ]);

        $report = Http::withToken($accessToken)->get('https://example.com/reports', ['page' => 1]);

        Arr::map(
            ['first' => 1, 'second' => 2, 'third' => 3, 'fourth' => 4, 'fifth' => 5, 'sixth' => 6],
            $callback,
            $flag,
        );
    }

    private function dueLabel(): string
    {
        return $this->invoice->due_date?->translatedFormat($this->tenantRecord()?->date_format ?? 'M j, Y')
            ?? __('notifications.soon');
    }
}
