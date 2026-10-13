<?php

app(SigningService::class)->sign(
    $this->signer,
    $provider,
);

$start = $startDate->copy()->setDate(
    $day->year,
    $day->month,
);

$sheet->getActiveSheet()->fromArray([
    [1, 2],
], null, 'A1');

return to_route('login')->with(
    'error',
    __('auth.account.not_pending'),
);

$total = app(SigningService::class)
    ->sign($this->signer, $provider, $anotherArgumentThatIsLong, $andOneMoreArgumentHere);

$response = Http::withToken($accessToken)
    ->get("https://api.example.com/v1.0/drives/{$driveId}/items/{$itemId}/details/full", [
        '$select' => 'name,size',
    ]);

expect(
    fn () => app(SigningService::class)->sign(
        $envelope->signers->first(),
        'credential',
    ),
)->toThrow(RuntimeException::class);

$items->each(
    fn ($item) => app(SigningService::class)
        ->sign(
            $item,
            'credential',
        ),
);

$result = $service->handle(
    fn ($item) => $item->first()->transform(
        $value,
    ),
);

$sheet->getActiveSheet()->fromArray([
    ['', 'Header', '', '', '', '', '', '', 'Budget', 'Actual'],
    ['', '1', 'Equipment', 'x', 'Quantity', 'Unit', 'Each', 'Budget', 'Actual', 'Notes', 'Extra'],
], null, 'A1');

$resultingCollectionName = process(
    fn ($item) => $item->firstRelatedModelRecordForTenantOrg()->transformIntoAnotherShapeAltogetherWithExtrasNow(
        $value,
    ),
);
