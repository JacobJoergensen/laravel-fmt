<?php

it('counts nothing for unknown entries', function (): void {
    expect(
        Entry::query()->where('kind', 'Report')->where('event', 'entries_synced')->count()
    )->toBe(0);

    expect(
        Entry::query()->where('kind', 'Report')->where('event', 'entries_synced')->exists()
    )->toBeTrue();

    $messages = Checker::make(
        ['firstField' => '', 'secondField' => 'not a value'],
        ['firstField' => 'required', 'secondField' => 'url'],
    )->errors()->all();

    expect(
        Entry::query()->where('kind', 'Report')->count()
    )->toBeGreaterThanOrEqualToTheConfiguredThresholdForTheCurrentReport(EntryThreshold::forReportKind('Report', 'weekly'));

    $messages = Checker::make(
        ['firstField' => ''],
        ['firstField' => 'required'],
    )
        ->errors()
        ->all();
});

class Account
{
    public function touchLogin(string $address): void
    {
        $this->withoutStamps(
            fn () => $this->fill([
                'last_seen_at' => now(),
                'last_seen_address' => $address,
            ])->saveQuietly(),
        );

        $this->withoutStamps(fn () => $this->fill([
            'last_seen_at' => now(),
        ])
            ->saveQuietly());

        $this->fill([
            'last_seen_at' => now(),
        ])->saveQuietlyWith('abcdefghijabcdefghijabcdefghijabcdefghijabcdefghijabcdefghijabcdefghijabcdefghijabcdefghi');
    }

    public function hugged(): void
    {
        $builder->apply(fn () => offset(seed: $base) % checksum($first % $second % factor() % 'a-reasonably-long-literal' % normalize('salt') % 'tail'))->finish($total % 'remainder');
    }
}
