<?php

it('pairs', function (): void {
    expect((float) $line->fresh()->rate)->toBe(1500.0)
        ->and((float) $line->fresh()->estimated_amount)->toBe(7500.0);

    expect($visibility['type'])->toBe('enum')->and($visibility['value'])->toBe('public')->and($visibility['options'])->not->toBeEmpty();

    expect($ids)->not->toContain($outside->id)->and($fresh->total_estimated_amount_for_the_ledger)->toBe(1950.0);

    expect($announced)
        ->toBe([1, 2])
        ->and($revision->parse_progress)
        ->toBe(100);

    expect($a)->toBe(1)->and($b)->toBe(2);

    $collection->filter(fn ($x) => $x->isActive())->and($other->firstWhereOrSomethingLongerThanTheLine())->values()->all();

    expect($this->validator->violations($member))->toHaveCount(1)->and($this->validator->violations($member)[0])->toContain('below the hourly wage minimum for the role in team');
});

it('keeps the expectation subject at the head indent', function (): void {
    expect(Writer::whereHas('entries', fn (EntryQueryBuilder $query) => $query->where('visible', true))->get()->pluck('slug')->all())->toBe(['first-writer'])
        ->and(Entry::whereRelation('writer', 'name', 'First Writer')->get())->toBeEmpty();
});

it('keeps a matcher with an author-broken array on its pair line', function (): void {
    expect($rows)->toHaveCount(4)
        ->and(entryDates($rows))->toBe([
            '2026-04-06',
            '2026-04-07',
        ]);

    expect($totals['days'])->toBe([
        'first' => 1,
        'second' => 2,
    ])
        ->and($totals['weeks'])->toBe([
            'first' => 1,
        ]);

    expect($rows)->toHaveCount(4)
        ->and(entryDates($rows))->toBe(['2026-04-06', '2026-04-07', '2026-04-08', '2026-04-09', '2026-04-10', '2026-04-11']);
});

it('keeps a matcher that breaks anyway on its pair line', function (): void {
    expect($rows)->toHaveCount(4)
        ->and($summary)->toBe(sprintf('%s entries between %s and %s for the %s workspace and %s team', $count, $first, $last, $workspaceName, $teamName));
});

it('ladders a run of matchers under its pair', function (): void {
    expect($statusCode)->toBe(1)
        ->and($output)
        ->toContain('  x '.join_paths('tests', 'Fixtures', 'fixers', 'method_casing_example_file.php'))
        ->toContain(<<<'DIFF'
              -    public function testItConvertsToSnakeCase()
            DIFF);
});
