<?php

class Panel
{
    public function open(): void
    {
        $this->send(
            'open-dialog',
            email: '',
            itemId: $this->item->id,
        )->to('dialog.invite');

        $component->send(
            'open-dialog',
            email: '',
        )->to('dialog.invite');

        $this->send(
            'open-dialog',
            email: '',
        )->to('dialog.invite')->with('extra');

        $this->send(
            'open-dialog',
            email: '',
        )->toTheTargetComponentWithAVeryLongName('dialog.invite.with.a.much.longer.identifier.value', $this->item->identifier, $this->item->owner);

        $this->send('open-dialog', email: '', itemId: $this->item->id, department: $this->item->department)->to('dialog.invite');

        $this->send('open-dialog', 'a message that is long enough to overflow the line even after the call moves down below its head', $this->item->identifier)->to('dialog.invite');

        $items->each(fn ($item) => $this
            ->send(
                'open-dialog',
                $item,
            )
            ->to('dialog.invite'));

        $this->send('open-dialog', fn ($item) => [
            'a' => $item,
        ])->to('dialog.invite');

        $this->send('open-dialog', fn ($item) => ['alpha' => $item->alpha, 'beta' => $item->beta, 'gamma' => $item->gamma])->to('dialog.invite');

        $this->send(
            'open-dialog',
            email: '',
        )->to('dialog.invite.dialog.invite.dialog.invite.dialog.invite.dialog.invite.dialog.invite.dialog.invite.dialo');

        $record
            ->attach(Upload::fake()
                ->withContent('notes.txt', "First line of the uploaded notes.\n\nSecond line of the uploaded notes."))
            ->toCollection('attachments');

        $items->each(fn ($item) => $this
            ->send(
                'open-dialog',
                $item,
            )->to('dialog.invite'));
    }

    public function middle(): array
    {
        $pinned = $this->searchedEntries()->filter(
            fn (Entry $entry): bool => $entry->isPinnedFor($user),
        )->values();

        $open = $board->entries()->reject(
            fn (Entry $entry): bool => $entry->isClosed(),
        )->values();

        return $this->searchedEntries()->filter(
            fn (Entry $entry): bool => $entry->isPinnedFor($user),
        )->values()->all();
    }
}
