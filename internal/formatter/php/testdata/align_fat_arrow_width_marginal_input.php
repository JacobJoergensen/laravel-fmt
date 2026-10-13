<?php

declare(strict_types=1);

class R
{
    public function r(object $item): array
    {
        return [
            'one' => $this->one,
            'description' => sprintf('%s: %s', $this->alpha ?? $this->bravo->charlie, $item->delta_echo_foxtrot),
            'a_key_of_length_twentyfive' => $this->two,
            'three' => $this->three,
        ];
    }

    public function payload(): array
    {
        return [
            'title' => 'Task list finalized',
            'action_url' => route('accounts.task-lists', [$this->taskList->account_id, 'expand' => $this->taskList->id]),
            'action_label' => 'View task list',
        ];
    }

    public function delete(User $user, Subscription $subscription): bool
    {
        return match ($subscription->type) {
            SubscriptionType::Direct => $subscription->participants()->exists(),
            SubscriptionType::Team => $subscription->team !== null && $user->can('deleteChannel', $subscription->team),
            SubscriptionType::Account => $subscription->account !== null,
        };
    }

    public function vatNumberFor(string $digits): ?string
    {
        return match ($this) {
            self::DK => strlen($digits) === 8 ? 'DK'.$digits : null,
            self::CZ => strlen($digits) >= 8 && strlen($digits) <= 10 ? 'CZ'.$digits : null,
            self::KR, self::US, self::GB, self::IE, self::CA, self::DE, self::HU => null,
        };
    }

    public function notify(): array
    {
        return [
            'title' => __('notifications.types.supplier_refund_approved.title', ['name' => $supplier?->name ?? '']),
            'notification_type' => 'supplier_refund_approved',
        ];
    }

    public function category(string $normalized): string
    {
        return match (true) {
            in_array($normalized, ['DAY', 'MORNING', 'AFTERNOON', 'NOON', 'MIDDAY', 'DAWN', 'SUNRISE'], true) => 'day',
            in_array($normalized, ['NIGHT', 'EVENING', 'DUSK', 'SUNSET', 'MIDNIGHT', 'LATE NIGHT'], true) => 'night',
            default => 'unknown',
        };
    }

    public function label(string $normalized): string
    {
        return match (true) {
            in_array($normalized, ['DAY', 'MORNING', 'AFTERNOON', 'NOON', 'MIDDAY', 'DAWN', 'SUNRISE'], true) => 'day',
            in_array($normalized, ['NIGHT', 'EVENING', 'DUSK'], true) => __('night'),
            default => 'unknown',
        };
    }

    public function nested(): void
    {
        register(
            schema([
                'a' => 1,
                'ccccccccccccccc' => $a->b()->c($this->d(Fooooooooooooooooooooooooooooooooooooooooooooooooooooo))->e(),
                'dddddddddddddddddd' => 2,
            ]),
        );
    }
}
