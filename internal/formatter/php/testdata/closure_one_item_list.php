<?php

function f()
{
    return Cache::remember("widgets.summary.report.cache.{$widgetGroupKey}", 300, function () use ($widgetGroupKey): array {
        return [];
    });
}

function g()
{
    $rows = Collection::make($items)->filter(function (\Acme\Models\SomeLongModelName $model) use ($widgetGroupKey): bool {
        return true;
    });

    $value = $condition
        ? null
        : $this->entries->first(function (\Acme\Models\Entry $entry) use ($thresholdValueWithAMuchLongerNameHereToo): bool {
            return true;
        });

    $value = $condition
        ? null
        : $this->entries->first(function (\Acme\Models\SomeVeryLongModelNameForTestingPurposes $entryInstanceValue): bool {
            return true;
        });

    DB::transaction(function (\Acme\Support\Transactions\TransactionContextWithAVeryLongName $context) use ($widget): \Acme\Support\Result {
        $widget->save();
    });

    DB::transaction(function () use ($widget, $label, $owner, $parentId, $folderId, $category): void {
        $widget->save();
    });

    it('describes a behaviour in a sentence that is long enough to overflow the line', function () {
        expect(true)->toBeTrue();
    });

    $callback = function () use (
        $value, // captured
    ) {
        return $value;
    };

    $items->each(function (
        // the model under test
        Model $model,
    ) {
        $model->touch();
    });
}

class Exporter
{
    public function export($user, $renderer, $scratchFilePath): void
    {
        $document = $this->withLocale($user->preferredLocale(), function () use ($renderer, $scratchFilePath): PdfDocument {
            return $renderer->store($scratchFilePath);
        });
    }
}
