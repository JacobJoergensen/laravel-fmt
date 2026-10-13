<?php

class Ledger
{
    public function transfer(Account $previous, Account $next): void
    {
        journal(EntryKind::Ledger->value)
            ->causedBy(Auth::user())
            ->performedOn($this)
            ->event('owner_transferred')
            ->log(sprintf(
                'Ownership of %s moved from the previous holder %s to the next holder %s',
                $this->name,
                $previous->name,
                $next->name,
            ));

        $entry
            ->causedBy(Auth::user())
            ->event('owner_transferred')
            ->log(sprintf(
                'Ownership of %s moved from the previous holder %s to the next holder %s',
                $this->name,
                $previous->name,
                $next->name,
            ));

        journal(EntryKind::Ledger->value)
            ->causedBy(Auth::user())
            ->performedOn($this)
            ->withProperties(['entry_id' => $entry->id, 'name' => $entry->name, 'status' => $entry->status->value, 'kind' => $entry->kind])
            ->event('entry_added')
            ->log("Added entry \"{$entry->name}\"");

        $entry->log(sprintf(
            'Ownership of %s moved from the previous holder %s to the next holder %s',
            $this->name,
            $previous->name,
        ))
            ->event('owner_transferred')
            ->save();

        journal(EntryKind::Ledger->value)
            ->causedBy(Auth::user())
            ->event('entry_added')
            ->withProperties(['entry_id' => $entry->id, 'name' => $entry->name, 'status' => $entry->status->value, 'kind' => $entry->kind]);

        journal(EntryKind::Ledger->value)
            ->causedBy(Auth::user())
            ->event('entry_added')
            ->recordTheEntryWithAnExtremelyLongMethodNamerecordTheEntryWithAnExtremelyLongMethodNamerecordTheEntryWithAnExtr($entry->id, $entry->name);
    }
}
