<?php

final class PublicFileController
{
    public function show($file, $share, $ledger, $weekStart, $weekEnd): void
    {
        $pdfContent = $this->renderingService->buildDecoratedReportOutput(
            $file,
            $share->creator,
            __('files.rendering.public_via_link', [
                'name' => $share->creator->name,
            ]),
        );

        $weekByAccount = $this->sumEntriesByAccount(
            $ledger,
            fn (Builder $query): Builder => $query->whereBetween('ledger_entries.date', [$weekStart, $weekEnd]),
        );

        Log::warning('Public file conversion failed for a document that was requested by the clients', [
            'file' => $file->id,
        ]);

        $user = User::query()->create([
            'name' => $share->name,
        ]);

        $total = sumAll($ledger, fn (Builder $query): Builder => $query->whereBetween('date', [$weekStart, $weekEnd]));

        $tracks = collect(range(1, 3))->map(fn (int $n): Track => Track::factory()->create([
            'record_id' => $record->id,
            'track_number' => (string) $n,
        ]));

        return $this->hostnameFrom(
            $this->send(fn (PendingRequest $request): Response => $request->post($this->endpoint(), $payload)),
        );

        warning(sprintf(
            'The rules enabled in your configuration require the following dependencies to be installed using [%s]: %s.',
            $manager->binary(),
            implode(', ', $missing),
        ));

        abort(1, sprintf(
            'The rules enabled in your configuration require the following dependencies to be installed: %s.',
            implode(', ', $missing),
        ));
    }
}
