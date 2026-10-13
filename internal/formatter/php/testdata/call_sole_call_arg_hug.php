<?php

class ReminderNotice
{
    public function toMail(): Message
    {
        return new Message()
            ->subject($this->headline())
            ->line(__('notices.types.reminder_notice.'.($this->daysRemaining > 0 ? 'mail_line_soonish' : 'mail_line_passed')))
            ->action('x');
    }

    public function build(): void
    {
        $message->attach(new Attachment('reports/monthly/'.$this->organization->slug.'/'.$this->period->format('Y-m').'/summary.pdf'));

        $message->line(__('notices.types.reminder_notice.'.($this->daysRemaining > 0 ? 'soon' : 'passed')));

        $message->line($builder->first()->label('notices.types.reminder_notice.'.($this->daysRemaining > 0 ? 'soon' : 'passed')));
    }

    public function filters(Builder $query, int $groupId): mixed
    {
        $query->whereHas('groups', static fn (\Vendor\Package\Contracts\Query\Builder $q) => $q->where(
            'groups.id',
            $groupId,
        ));

        $query->whereHas('groups', static fn (\Vendor\Package\Contracts\Query\Builder $q) => $q->where('groups.id', $groupId));

        $rows = $query->get()
            ->map(static fn (self $row): string => ($row->owner?->display_name ?? __('common.owner')).': '.__($row->position > 1 ? 'some.conflicts.position' : 'some.conflicts.single', ['position' => $row->position]))
            ->all();

        $q->where(fn ($inner) => $inner->whereLike('first_name', $wordLike)->orWhereLike('last_name', $wordLike)->orWhereLike('email', $wordLike));

        $rows->each(fn ($row) => $this->firstVeryLongPropertyName->secondVeryLongPropertyName->thirdVeryLongPropertyNameThatOverflows);

        $items = collect($this->source->items(RangeScope::project($project->id), ImmutableMoment::parse('2026-05-01'), ImmutableMoment::parse('2026-05-31')));

        return $sorted->groupBy(fn (Permission $permission) => Permissions::tryFrom($permission->name)?->group() ?? __('enums.permission_groups.other'));
    }

    public function picks(array $list): void
    {
        $first = $list->first(fn (self $x): bool => stripos($name, $x->longnamexy) !== false || stripos($x->longname, $name) !== false);
        $other = Pick::first(fn (self $x): bool => stripos($name, $x->longnamexy) !== false || stripos($x->longname, $name) !== false);
    }
}
